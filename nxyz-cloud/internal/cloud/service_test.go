package cloud

import "testing"

func TestPublishedServiceLifecycle(t *testing.T) {
	cp, err := New("")
	if err != nil {
		t.Fatal(err)
	}
	_, err = cp.RegisterNode(RegisterNodeRequest{ID: "node-a", Name: "alpha", Address: "127.0.0.1", CPU: 2000, Memory: 2048, Disk: 100000})
	if err != nil {
		t.Fatal(err)
	}
	w, err := cp.CreateWorkload(CreateWorkloadRequest{Name: "web", Image: "nginx:alpine", CPU: 250, Memory: 128, ContainerPort: 80, HealthPath: "/"})
	if err != nil {
		t.Fatal(err)
	}
	if !w.Publish || w.ContainerPort != 80 || w.Health != "starting" {
		t.Fatalf("unexpected published workload: %+v", w)
	}
	w, err = cp.UpdateWorkloadStatus(w.ID, UpdateWorkloadStatusRequest{NodeID: w.NodeID, Status: "running", HostPort: 49152, Endpoint: "http://127.0.0.1:49152", Health: "healthy"})
	if err != nil {
		t.Fatal(err)
	}
	if w.Endpoint == "" || w.HostPort != 49152 || w.Health != "healthy" {
		t.Fatalf("service endpoint not persisted: %+v", w)
	}
	services := cp.ListServices()
	if len(services) != 1 || services[0].Name != "web" {
		t.Fatalf("unexpected services: %+v", services)
	}
	found, err := cp.FindService("web")
	if err != nil || found.ID != w.ID {
		t.Fatalf("find service: %+v %v", found, err)
	}
	s := cp.Summary()
	if s.PublishedServices != 1 || s.RunningWorkloads != 1 || s.TotalDisk != 100000 {
		t.Fatalf("unexpected summary: %+v", s)
	}
}

func TestPublishedWorkloadRequiresPort(t *testing.T) {
	cp, _ := New("")
	_, _ = cp.RegisterNode(RegisterNodeRequest{ID: "a", Name: "alpha", CPU: 1000, Memory: 1024})
	if _, err := cp.CreateWorkload(CreateWorkloadRequest{Name: "bad", Image: "nginx", CPU: 100, Memory: 64, Publish: true}); err == nil {
		t.Fatal("expected published workload without a port to fail")
	}
}

func TestTerminalServiceClearsEndpoint(t *testing.T) {
	cp, _ := New("")
	_, _ = cp.RegisterNode(RegisterNodeRequest{ID: "a", Name: "alpha", CPU: 1000, Memory: 1024})
	w, _ := cp.CreateWorkload(CreateWorkloadRequest{Name: "web", Image: "nginx", CPU: 100, Memory: 64, ContainerPort: 80})
	w, _ = cp.UpdateWorkloadStatus(w.ID, UpdateWorkloadStatusRequest{NodeID: w.NodeID, Status: "running", HostPort: 40000, Endpoint: "http://127.0.0.1:40000", Health: "unknown"})
	w, _ = cp.UpdateWorkloadStatus(w.ID, UpdateWorkloadStatusRequest{NodeID: w.NodeID, Status: "failed", Health: "unhealthy"})
	if w.Endpoint != "" || w.HostPort != 0 {
		t.Fatalf("terminal service retained stale endpoint: %+v", w)
	}
}
