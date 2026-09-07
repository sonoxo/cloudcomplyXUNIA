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
	CapacityDisk   int       `json:"capacity_disk_mb,omitempty"`
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
	ContainerPort  int       `json:"container_port,omitempty"`
	Publish        bool      `json:"publish,omitempty"`
	HostPort       int       `json:"host_port,omitempty"`
	Endpoint       string    `json:"endpoint,omitempty"`
	HealthPath     string    `json:"health_path,omitempty"`
	Health         string    `json:"health,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type CreateWorkloadRequest struct {
	Name          string `json:"name"`
	Image         string `json:"image"`
	CPU           int    `json:"cpu_millicores"`
	Memory        int    `json:"memory_mb"`
	ContainerPort int    `json:"container_port,omitempty"`
	Publish       bool   `json:"publish,omitempty"`
	HealthPath    string `json:"health_path,omitempty"`
}

type UpdateWorkloadStatusRequest struct {
	NodeID   string `json:"node_id"`
	Status   string `json:"status"`
	Message  string `json:"message,omitempty"`
	HostPort int    `json:"host_port,omitempty"`
	Endpoint string `json:"endpoint,omitempty"`
	Health   string `json:"health,omitempty"`
}

type RegisterNodeRequest struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Address string `json:"address,omitempty"`
	CPU     int    `json:"capacity_cpu_millicores"`
	Memory  int    `json:"capacity_memory_mb"`
	Disk    int    `json:"capacity_disk_mb,omitempty"`
}

type Summary struct {
	Name              string `json:"name"`
	Version           string `json:"version"`
	UptimeSeconds     int64  `json:"uptime_seconds"`
	Nodes             int    `json:"nodes"`
	HealthyNodes      int    `json:"healthy_nodes"`
	Workloads         int    `json:"workloads"`
	RunningWorkloads  int    `json:"running_workloads"`
	PublishedServices int    `json:"published_services"`
	AllocatedCPU      int    `json:"allocated_cpu_millicores"`
	TotalCPU          int    `json:"total_cpu_millicores"`
	AllocatedMemory   int    `json:"allocated_memory_mb"`
	TotalMemory       int    `json:"total_memory_mb"`
	TotalDisk         int    `json:"total_disk_mb"`
	CompliancePlane   string `json:"compliance_plane"`
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
	req.ID = strings.TrimSpace(req.ID)
	req.Name = strings.TrimSpace(req.Name)
	req.Address = strings.TrimSpace(req.Address)
	if req.ID == "" || req.Name == "" {
		return nil, errors.New("node id and name are required")
	}
	if req.CPU <= 0 || req.Memory <= 0 {
		return nil, errors.New("node capacity must be greater than zero")
	}
	if req.Disk < 0 {
		return nil, errors.New("node disk capacity cannot be negative")
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
	n.CapacityDisk = req.Disk
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
	return c.listWorkloadsLocked(false)
}

func (c *ControlPlane) ListServices() []Workload {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.listWorkloadsLocked(true)
}

func (c *ControlPlane) listWorkloadsLocked(servicesOnly bool) []Workload {
	out := make([]Workload, 0, len(c.workloads))
	for _, w := range c.workloads {
		if servicesOnly && !w.Publish {
			continue
		}
		out = append(out, *w)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out
}

func (c *ControlPlane) GetWorkload(id string) (*Workload, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	w, ok := c.workloads[id]
	if !ok {
		return nil, ErrNotFound
	}
	cp := *w
	return &cp, nil
}

func (c *ControlPlane) FindService(name string) (*Workload, error) {
	name = strings.TrimSpace(name)
	c.mu.RLock()
	defer c.mu.RUnlock()
	var best *Workload
	for _, w := range c.workloads {
		if w.Publish && w.Name == name {
			if best == nil || (w.Status == "running" && best.Status != "running") || w.CreatedAt.After(best.CreatedAt) {
				best = w
			}
		}
	}
	if best == nil {
		return nil, ErrNotFound
	}
	cp := *best
	return &cp, nil
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
	req.HealthPath = strings.TrimSpace(req.HealthPath)
	if req.Name == "" || req.Image == "" {
		return nil, errors.New("workload name and image are required")
	}
	if req.CPU <= 0 || req.Memory <= 0 {
		return nil, errors.New("cpu_millicores and memory_mb must be greater than zero")
	}
	if req.ContainerPort < 0 || req.ContainerPort > 65535 {
		return nil, errors.New("container_port must be between 1 and 65535")
	}
	if req.ContainerPort > 0 {
		req.Publish = true
	}
	if req.Publish && req.ContainerPort == 0 {
		return nil, errors.New("published workload requires container_port")
	}
	if req.HealthPath != "" && !strings.HasPrefix(req.HealthPath, "/") {
		return nil, errors.New("health_path must begin with /")
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	node := c.selectNodeLocked(req.CPU, req.Memory)
	if node == nil {
		return nil, ErrNoCapacity
	}
	now := c.now()
	id := fmt.Sprintf("w-%d", now.UnixNano())
	health := ""
	if req.Publish {
		health = "starting"
	}
	w := &Workload{
		ID: id, Name: req.Name, Image: req.Image, CPU: req.CPU, Memory: req.Memory,
		NodeID: node.ID, Status: "scheduled", ContainerPort: req.ContainerPort,
		Publish: req.Publish, HealthPath: req.HealthPath, Health: health,
		CreatedAt: now, UpdatedAt: now,
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
	req.Endpoint = strings.TrimSpace(req.Endpoint)
	req.Health = strings.ToLower(strings.TrimSpace(req.Health))
	if req.NodeID == "" {
		return nil, errors.New("node_id is required")
	}
	if !validWorkloadStatus(req.Status) {
		return nil, errors.New("status must be scheduled, running, succeeded, or failed")
	}
	if req.HostPort < 0 || req.HostPort > 65535 {
		return nil, errors.New("host_port must be between 1 and 65535")
	}
	if req.Health != "" && !validHealth(req.Health) {
		return nil, errors.New("health must be starting, healthy, unhealthy, or unknown")
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
	if req.HostPort > 0 {
		w.HostPort = req.HostPort
	}
	if req.Endpoint != "" {
		w.Endpoint = req.Endpoint
	}
	if req.Health != "" {
		w.Health = req.Health
	}
	if req.Status == "succeeded" || req.Status == "failed" {
		w.HostPort = 0
		w.Endpoint = ""
		if w.Publish && w.Health != "unhealthy" {
			w.Health = "unknown"
		}
	}
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
	s := Summary{Name: "NXYZ Cloud", Version: "1.0.0", UptimeSeconds: int64(c.now().Sub(c.startedAt).Seconds()), CompliancePlane: "cloudcomplyXUNIA bridge-ready"}
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
		s.TotalDisk += n.CapacityDisk
	}
	for _, w := range c.workloads {
		if w.Status == "running" {
			s.RunningWorkloads++
		}
		if w.Publish && w.Endpoint != "" && w.Status == "running" {
			s.PublishedServices++
		}
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

func validHealth(health string) bool {
	switch health {
	case "starting", "healthy", "unhealthy", "unknown":
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
