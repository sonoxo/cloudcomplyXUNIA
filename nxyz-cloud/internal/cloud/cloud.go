package cloud

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

var (
	ErrNoCapacity = errors.New("no healthy node has enough capacity")
	ErrNotFound   = errors.New("resource not found")
	ErrForbidden  = errors.New("node is not authorized for this workload")
)

type Node struct {
	ID             string    `json:"id"`
	Name           string    `json:"name"`
	Address        string    `json:"address,omitempty"`
	CapacityCPU    int       `json:"capacity_cpu_millicores"`
	CapacityMemory int       `json:"capacity_memory_mb"`
	UsedCPU        int       `json:"used_cpu_millicores"`
	UsedMemory     int       `json:"used_memory_mb"`
	Status         string    `json:"status"`
	LastSeen       time.Time `json:"last_seen"`
}

type Workload struct {
	ID             string    `json:"id"`
	Name           string    `json:"name"`
	Image          string    `json:"image"`
	CPU            int       `json:"cpu_millicores"`
	Memory         int       `json:"memory_mb"`
	NodeID         string    `json:"node_id"`
	Status         string    `json:"status"`
	RuntimeMessage string    `json:"runtime_message,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type CreateWorkloadRequest struct {
	Name   string `json:"name"`
	Image  string `json:"image"`
	CPU    int    `json:"cpu_millicores"`
	Memory int    `json:"memory_mb"`
}

type UpdateWorkloadStatusRequest struct {
	NodeID  string `json:"node_id"`
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

type RegisterNodeRequest struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Address string `json:"address,omitempty"`
	CPU     int    `json:"capacity_cpu_millicores"`
	Memory  int    `json:"capacity_memory_mb"`
}

type Summary struct {
	Name            string `json:"name"`
	Version         string `json:"version"`
	UptimeSeconds   int64  `json:"uptime_seconds"`
	Nodes           int    `json:"nodes"`
	HealthyNodes    int    `json:"healthy_nodes"`
	Workloads       int    `json:"workloads"`
	AllocatedCPU    int    `json:"allocated_cpu_millicores"`
	TotalCPU        int    `json:"total_cpu_millicores"`
	AllocatedMemory int    `json:"allocated_memory_mb"`
	TotalMemory     int    `json:"total_memory_mb"`
	CompliancePlane string `json:"compliance_plane"`
}

type persistedState struct {
	Nodes     map[string]*Node     `json:"nodes"`
	Workloads map[string]*Workload `json:"workloads"`
}

type ControlPlane struct {
	mu        sync.RWMutex
	nodes     map[string]*Node
	workloads map[string]*Workload
	statePath string
	startedAt time.Time
	now       func() time.Time
}

func New(statePath string) (*ControlPlane, error) {
	cp := &ControlPlane{
		nodes:     map[string]*Node{},
		workloads: map[string]*Workload{},
		statePath: statePath,
		startedAt: time.Now().UTC(),
		now:       func() time.Time { return time.Now().UTC() },
	}
	if statePath != "" {
		if err := cp.load(); err != nil {
			return nil, err
		}
	}
	return cp, nil
}

func (c *ControlPlane) SetClock(now func() time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = now
}

func (c *ControlPlane) RegisterNode(req RegisterNodeRequest) (*Node, error) {
	if strings.TrimSpace(req.ID) == "" || strings.TrimSpace(req.Name) == "" {
		return nil, errors.New("node id and name are required")
	}
	if req.CPU <= 0 || req.Memory <= 0 {
		return nil, errors.New("node capacity must be greater than zero")
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	now := c.now()
	n, exists := c.nodes[req.ID]
	if !exists {
		n = &Node{ID: req.ID}
		c.nodes[req.ID] = n
	}
	n.Name = req.Name
	n.Address = req.Address
	n.CapacityCPU = req.CPU
	n.CapacityMemory = req.Memory
	n.Status = "healthy"
	n.LastSeen = now
	c.recalculateNodeUsageLocked(req.ID)
	if err := c.persistLocked(); err != nil {
		return nil, err
	}
	cp := *n
	return &cp, nil
}

func (c *ControlPlane) HeartbeatNode(id string) (*Node, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	n, ok := c.nodes[id]
	if !ok {
		return nil, ErrNotFound
	}
	n.Status = "healthy"
	n.LastSeen = c.now()
	if err := c.persistLocked(); err != nil {
		return nil, err
	}
	cp := *n
	return &cp, nil
}

func (c *ControlPlane) ReapStaleNodes(maxAge time.Duration) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	cutoff := c.now().Add(-maxAge)
	changed := 0
	for _, n := range c.nodes {
		if n.LastSeen.Before(cutoff) && n.Status != "offline" {
			n.Status = "offline"
			changed++
		}
	}
	if changed > 0 {
		_ = c.persistLocked()
	}
	return changed
}

func (c *ControlPlane) ListNodes() []Node {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]Node, 0, len(c.nodes))
	for _, n := range c.nodes {
		out = append(out, *n)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func (c *ControlPlane) ListWorkloads() []Workload {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]Workload, 0, len(c.workloads))
	for _, w := range c.workloads {
		out = append(out, *w)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out
}

func (c *ControlPlane) ListNodeWorkloads(nodeID string) ([]Workload, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if _, ok := c.nodes[nodeID]; !ok {
		return nil, ErrNotFound
	}
	out := make([]Workload, 0)
	for _, w := range c.workloads {
		if w.NodeID == nodeID {
			out = append(out, *w)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}

func (c *ControlPlane) CreateWorkload(req CreateWorkloadRequest) (*Workload, error) {
	req.Name = strings.TrimSpace(req.Name)
	req.Image = strings.TrimSpace(req.Image)
	if req.Name == "" || req.Image == "" {
		return nil, errors.New("workload name and image are required")
	}
	if req.CPU <= 0 || req.Memory <= 0 {
		return nil, errors.New("cpu_millicores and memory_mb must be greater than zero")
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	node := c.selectNodeLocked(req.CPU, req.Memory)
	if node == nil {
		return nil, ErrNoCapacity
	}
	now := c.now()
	id := fmt.Sprintf("w-%d", now.UnixNano())
	w := &Workload{
		ID: id, Name: req.Name, Image: req.Image, CPU: req.CPU, Memory: req.Memory,
		NodeID: node.ID, Status: "scheduled", CreatedAt: now, UpdatedAt: now,
	}
	c.workloads[id] = w
	node.UsedCPU += req.CPU
	node.UsedMemory += req.Memory
	if err := c.persistLocked(); err != nil {
		delete(c.workloads, id)
		node.UsedCPU -= req.CPU
		node.UsedMemory -= req.Memory
		return nil, err
	}
	cp := *w
	return &cp, nil
}

func (c *ControlPlane) UpdateWorkloadStatus(id string, req UpdateWorkloadStatusRequest) (*Workload, error) {
	req.NodeID = strings.TrimSpace(req.NodeID)
	req.Status = strings.ToLower(strings.TrimSpace(req.Status))
	if req.NodeID == "" {
		return nil, errors.New("node_id is required")
	}
	if !validWorkloadStatus(req.Status) {
		return nil, errors.New("status must be scheduled, running, succeeded, or failed")
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	w, ok := c.workloads[id]
	if !ok {
		return nil, ErrNotFound
	}
	if w.NodeID != req.NodeID {
		return nil, ErrForbidden
	}
	w.Status = req.Status
	w.RuntimeMessage = strings.TrimSpace(req.Message)
	w.UpdatedAt = c.now()
	c.recalculateNodeUsageLocked(w.NodeID)
	if err := c.persistLocked(); err != nil {
		return nil, err
	}
	cp := *w
	return &cp, nil
}

func (c *ControlPlane) DeleteWorkload(id string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	w, ok := c.workloads[id]
	if !ok {
		return ErrNotFound
	}
	delete(c.workloads, id)
	c.recalculateNodeUsageLocked(w.NodeID)
	return c.persistLocked()
}

func (c *ControlPlane) Summary() Summary {
	c.mu.RLock()
	defer c.mu.RUnlock()
	s := Summary{Name: "NXYZ Cloud", Version: "0.2.0", UptimeSeconds: int64(c.now().Sub(c.startedAt).Seconds()), CompliancePlane: "cloudcomplyXUNIA bridge-ready"}
	s.Nodes = len(c.nodes)
	s.Workloads = len(c.workloads)
	for _, n := range c.nodes {
		if n.Status == "healthy" {
			s.HealthyNodes++
		}
		s.AllocatedCPU += n.UsedCPU
		s.TotalCPU += n.CapacityCPU
		s.AllocatedMemory += n.UsedMemory
		s.TotalMemory += n.CapacityMemory
	}
	return s
}

func (c *ControlPlane) selectNodeLocked(cpu, memory int) *Node {
	var best *Node
	bestScore := 2.0
	for _, n := range c.nodes {
		if n.Status != "healthy" || n.CapacityCPU-n.UsedCPU < cpu || n.CapacityMemory-n.UsedMemory < memory {
			continue
		}
		cpuRatio := float64(n.UsedCPU+cpu) / float64(n.CapacityCPU)
		memRatio := float64(n.UsedMemory+memory) / float64(n.CapacityMemory)
		score := (cpuRatio + memRatio) / 2
		if best == nil || score < bestScore || (score == bestScore && n.ID < best.ID) {
			best = n
			bestScore = score
		}
	}
	return best
}

func (c *ControlPlane) recalculateNodeUsageLocked(nodeID string) {
	n, ok := c.nodes[nodeID]
	if !ok {
		return
	}
	n.UsedCPU, n.UsedMemory = 0, 0
	for _, w := range c.workloads {
		if w.NodeID == nodeID && workloadConsumesCapacity(w.Status) {
			n.UsedCPU += w.CPU
			n.UsedMemory += w.Memory
		}
	}
}

func validWorkloadStatus(status string) bool {
	switch status {
	case "scheduled", "running", "succeeded", "failed":
		return true
	default:
		return false
	}
}

func workloadConsumesCapacity(status string) bool {
	return status == "scheduled" || status == "running"
}

func (c *ControlPlane) load() error {
	b, err := os.ReadFile(c.statePath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read state: %w", err)
	}
	if len(b) == 0 {
		return nil
	}
	var s persistedState
	if err := json.Unmarshal(b, &s); err != nil {
		return fmt.Errorf("decode state: %w", err)
	}
	if s.Nodes != nil {
		c.nodes = s.Nodes
	}
	if s.Workloads != nil {
		c.workloads = s.Workloads
	}
	for id := range c.nodes {
		c.recalculateNodeUsageLocked(id)
	}
	return nil
}

func (c *ControlPlane) persistLocked() error {
	if c.statePath == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(c.statePath), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(persistedState{Nodes: c.nodes, Workloads: c.workloads}, "", "  ")
	if err != nil {
		return err
	}
	tmp := c.statePath + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, c.statePath)
}
