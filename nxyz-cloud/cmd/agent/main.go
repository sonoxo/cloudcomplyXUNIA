package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/sonoxo/nxyz-cloud/internal/cloud"
)

type runtimeConfig struct {
	Binary            string
	AllowedRegistries map[string]struct{}
	PublishBind       string
	PublishHost       string
}

func main() {
	cp := strings.TrimRight(env("NXYZ_CONTROL_PLANE", "http://controlplane:8080"), "/")
	id := env("NXYZ_NODE_ID", hostname())
	token := strings.TrimSpace(os.Getenv("NXYZ_CLUSTER_TOKEN"))
	nodeAddress := env("NXYZ_NODE_ADDRESS", "127.0.0.1")
	req := cloud.RegisterNodeRequest{
		ID:      id,
		Name:    env("NXYZ_NODE_NAME", id),
		Address: nodeAddress,
		CPU:     envInt("NXYZ_NODE_CPU", 2000),
		Memory:  envInt("NXYZ_NODE_MEMORY_MB", 2048),
		Disk:    envInt("NXYZ_NODE_DISK_MB", 0),
	}

	client := &http.Client{Timeout: 8 * time.Second}
	for {
		if err := register(client, cp, token, req); err == nil {
			break
		} else {
			log.Printf("register failed: %v", err)
		}
		time.Sleep(2 * time.Second)
	}
	log.Printf("🛰️  NXYZ agent %s registered with %s", id, cp)

	runtime, executionEnabled := discoverRuntime(nodeAddress)
	if executionEnabled {
		log.Printf("📦 rootless OCI execution enabled with %s", runtime.Binary)
		log.Printf("🌐 service publishing bind=%s host=%s", runtime.PublishBind, runtime.PublishHost)
	} else {
		log.Printf("ℹ️ OCI execution is scheduler-only on this node; install Podman or set NXYZ_EXECUTION_ENABLED=false explicitly")
	}

	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()
	for {
		if err := heartbeat(client, cp, token, id); err != nil {
			log.Printf("heartbeat failed: %v; re-registering", err)
			_ = register(client, cp, token, req)
		}
		if executionEnabled {
			if err := reconcile(client, cp, token, id, runtime); err != nil {
				log.Printf("reconcile failed: %v", err)
			}
		}
		<-ticker.C
	}
}

func register(client *http.Client, cp, token string, req cloud.RegisterNodeRequest) error {
	return doJSON(client, http.MethodPost, cp+"/api/v1/nodes/register", token, req, nil)
}

func heartbeat(client *http.Client, cp, token, id string) error {
	return doJSON(client, http.MethodPost, cp+"/api/v1/nodes/"+id+"/heartbeat", token, nil, nil)
}

func reconcile(client *http.Client, cp, token, nodeID string, runtime runtimeConfig) error {
	var workloads []cloud.Workload
	if err := doJSON(client, http.MethodGet, cp+"/api/v1/nodes/"+nodeID+"/workloads", token, nil, &workloads); err != nil {
		return err
	}

	desired := make(map[string]struct{})
	for _, w := range workloads {
		name := containerName(w.ID)
		switch w.Status {
		case "scheduled":
			desired[name] = struct{}{}
			hostPort, endpoint, err := ensureRunning(runtime, nodeID, w)
			if err != nil {
				_ = reportStatus(client, cp, token, w.ID, nodeID, "failed", err.Error(), 0, "", "unhealthy")
				continue
			}
			health := ""
			if w.Publish {
				health = probeHealth(client, endpoint, w.HealthPath)
			}
			_ = reportStatus(client, cp, token, w.ID, nodeID, "running", "rootless Podman container is running", hostPort, endpoint, health)
		case "running":
			desired[name] = struct{}{}
			status, exitCode, err := inspect(runtime, name)
			if err != nil {
				hostPort, endpoint, startErr := ensureRunning(runtime, nodeID, w)
				if startErr != nil {
					_ = reportStatus(client, cp, token, w.ID, nodeID, "failed", startErr.Error(), 0, "", "unhealthy")
				} else if w.Publish {
					health := probeHealth(client, endpoint, w.HealthPath)
					_ = reportStatus(client, cp, token, w.ID, nodeID, "running", "container recovered", hostPort, endpoint, health)
				}
				continue
			}
			switch status {
			case "running", "created":
				if w.Publish {
					hostPort, endpoint, portErr := publishedEndpoint(runtime, name, w.ContainerPort)
					if portErr != nil {
						_ = reportStatus(client, cp, token, w.ID, nodeID, "running", portErr.Error(), 0, "", "unhealthy")
						continue
					}
					health := probeHealth(client, endpoint, w.HealthPath)
					if endpoint != w.Endpoint || hostPort != w.HostPort || health != w.Health {
						_ = reportStatus(client, cp, token, w.ID, nodeID, "running", "service endpoint reconciled", hostPort, endpoint, health)
					}
				}
			case "exited":
				if exitCode == 0 {
					_ = reportStatus(client, cp, token, w.ID, nodeID, "succeeded", "container exited successfully", 0, "", "unknown")
				} else {
					_ = reportStatus(client, cp, token, w.ID, nodeID, "failed", fmt.Sprintf("container exited with code %d", exitCode), 0, "", "unhealthy")
				}
			default:
				_ = reportStatus(client, cp, token, w.ID, nodeID, "failed", "container entered state "+status, 0, "", "unhealthy")
			}
		}
	}
	return garbageCollect(runtime, nodeID, desired)
}

func ensureRunning(runtime runtimeConfig, nodeID string, w cloud.Workload) (int, string, error) {
	name := containerName(w.ID)
	if status, _, err := inspect(runtime, name); err == nil && (status == "running" || status == "created") {
		if w.Publish {
			return publishedEndpoint(runtime, name, w.ContainerPort)
		}
		return 0, "", nil
	}
	image, err := normalizeImage(w.Image)
	if err != nil {
		return 0, "", err
	}
	registry := strings.SplitN(image, "/", 2)[0]
	if _, ok := runtime.AllowedRegistries[registry]; !ok {
		return 0, "", fmt.Errorf("registry %q is not allowed", registry)
	}

	cpus := fmt.Sprintf("%.3f", float64(w.CPU)/1000.0)
	args := []string{"run", "-d", "--replace", "--pull=missing"}
	if registry == "localhost" || strings.HasPrefix(registry, "localhost:") {
		args = append(args, "--tls-verify=false")
	}
	if w.Publish {
		if w.ContainerPort <= 0 || w.ContainerPort > 65535 {
			return 0, "", errors.New("published workload has invalid container port")
		}
		args = append(args, "-p", fmt.Sprintf("%s::%d", runtime.PublishBind, w.ContainerPort))
	}
	args = append(args,
		"--name", name,
		"--cpus", cpus,
		"--memory", fmt.Sprintf("%dm", w.Memory),
		"--pids-limit", "256",
		"--security-opt", "no-new-privileges",
		"--network", "private",
		"--label", "nxyz.managed=true",
		"--label", "nxyz.workload-id="+w.ID,
		"--label", "nxyz.node-id="+nodeID,
		image,
	)
	cmd := exec.Command(runtime.Binary, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return 0, "", fmt.Errorf("%s run failed: %w: %s", runtime.Binary, err, strings.TrimSpace(string(out)))
	}
	if w.Publish {
		return publishedEndpoint(runtime, name, w.ContainerPort)
	}
	return 0, "", nil
}

func publishedEndpoint(runtime runtimeConfig, name string, containerPort int) (int, string, error) {
	cmd := exec.Command(runtime.Binary, "port", name, fmt.Sprintf("%d/tcp", containerPort))
	out, err := cmd.CombinedOutput()
	if err != nil {
		return 0, "", fmt.Errorf("discover published port: %w: %s", err, strings.TrimSpace(string(out)))
	}
	line := strings.TrimSpace(strings.SplitN(string(out), "\n", 2)[0])
	if line == "" {
		return 0, "", errors.New("runtime did not report a published port")
	}
	_, portText, err := net.SplitHostPort(line)
	if err != nil {
		idx := strings.LastIndex(line, ":")
		if idx < 0 {
			return 0, "", fmt.Errorf("unexpected port mapping %q", line)
		}
		portText = line[idx+1:]
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port <= 0 {
		return 0, "", fmt.Errorf("invalid published port %q", portText)
	}
	host := strings.TrimSpace(runtime.PublishHost)
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	return port, fmt.Sprintf("http://%s:%d", host, port), nil
}

func probeHealth(client *http.Client, endpoint, path string) string {
	if endpoint == "" {
		return "starting"
	}
	if strings.TrimSpace(path) == "" {
		return "unknown"
	}
	url := strings.TrimRight(endpoint, "/") + path
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return "unhealthy"
	}
	resp, err := client.Do(req)
	if err != nil {
		return "unhealthy"
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 200 && resp.StatusCode < 400 {
		return "healthy"
	}
	return "unhealthy"
}

func inspect(runtime runtimeConfig, name string) (string, int, error) {
	cmd := exec.Command(runtime.Binary, "inspect", "--format", "{{.State.Status}}|{{.State.ExitCode}}", name)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", 0, err
	}
	parts := strings.Split(strings.TrimSpace(string(out)), "|")
	if len(parts) != 2 {
		return "", 0, errors.New("unexpected container inspect response")
	}
	code, err := strconv.Atoi(parts[1])
	if err != nil {
		return "", 0, err
	}
	return parts[0], code, nil
}

func garbageCollect(runtime runtimeConfig, nodeID string, desired map[string]struct{}) error {
	cmd := exec.Command(runtime.Binary, "ps", "-a", "--filter", "label=nxyz.managed=true", "--filter", "label=nxyz.node-id="+nodeID, "--format", "{{.Names}}")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("list managed containers: %w: %s", err, strings.TrimSpace(string(out)))
	}
	for _, name := range strings.Fields(string(out)) {
		if _, keep := desired[name]; keep {
			continue
		}
		remove := exec.Command(runtime.Binary, "rm", "-f", name)
		if b, err := remove.CombinedOutput(); err != nil {
			return fmt.Errorf("remove stale container %s: %w: %s", name, err, strings.TrimSpace(string(b)))
		}
	}
	return nil
}

func reportStatus(client *http.Client, cp, token, workloadID, nodeID, status, message string, hostPort int, endpoint, health string) error {
	req := cloud.UpdateWorkloadStatusRequest{NodeID: nodeID, Status: status, Message: message, HostPort: hostPort, Endpoint: endpoint, Health: health}
	return doJSON(client, http.MethodPost, cp+"/api/v1/workloads/"+workloadID+"/status", token, req, nil)
}

func doJSON(client *http.Client, method, url, token string, body any, dst any) error {
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, url, reader)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		data, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("status %s: %s", resp.Status, strings.TrimSpace(string(data)))
	}
	if dst != nil {
		if err := json.NewDecoder(resp.Body).Decode(dst); err != nil {
			return err
		}
	}
	return nil
}

func discoverRuntime(nodeAddress string) (runtimeConfig, bool) {
	allowed := map[string]struct{}{}
	for _, item := range strings.Split(env("NXYZ_ALLOWED_REGISTRIES", "docker.io,ghcr.io,quay.io,localhost:5000"), ",") {
		if item = strings.TrimSpace(item); item != "" {
			allowed[item] = struct{}{}
		}
	}
	cfg := runtimeConfig{
		Binary:            env("NXYZ_RUNTIME", "podman"),
		AllowedRegistries: allowed,
		PublishBind:       env("NXYZ_PUBLISH_BIND", "127.0.0.1"),
		PublishHost:       env("NXYZ_PUBLISH_HOST", nodeAddress),
	}
	mode := strings.ToLower(env("NXYZ_EXECUTION_ENABLED", "auto"))
	if mode == "false" || mode == "0" || mode == "off" {
		return cfg, false
	}
	if _, err := exec.LookPath(cfg.Binary); err != nil {
		if mode == "true" || mode == "1" || mode == "on" {
			log.Fatalf("NXYZ execution requested but runtime %q was not found: %v", cfg.Binary, err)
		}
		return cfg, false
	}
	return cfg, true
}

func normalizeImage(image string) (string, error) {
	image = strings.TrimSpace(image)
	if image == "" || strings.ContainsAny(image, " \t\r\n") || strings.Contains(image, "://") || strings.HasPrefix(image, "-") {
		return "", errors.New("invalid OCI image reference")
	}
	first, _, hasSlash := strings.Cut(image, "/")
	if !hasSlash {
		return "docker.io/library/" + image, nil
	}
	if strings.Contains(first, ".") || strings.Contains(first, ":") || first == "localhost" {
		return image, nil
	}
	return "docker.io/" + image, nil
}

func containerName(workloadID string) string {
	var b strings.Builder
	b.WriteString("nxyz-")
	for _, r := range workloadID {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' {
			b.WriteRune(r)
		} else {
			b.WriteByte('-')
		}
	}
	return b.String()
}

func env(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func envInt(key string, fallback int) int {
	v, err := strconv.Atoi(strings.TrimSpace(os.Getenv(key)))
	if err == nil && v >= 0 {
		return v
	}
	return fallback
}

func hostname() string {
	h, err := os.Hostname()
	if err != nil || h == "" {
		return "nxyz-node"
	}
	return h
}
