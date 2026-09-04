package cloud

import (
	"errors"
	"testing"
)

func TestSchedulesOntoLeastLoadedHealthyNode(t *testing.T) {
	cp, err := New("")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cp.RegisterNode(RegisterNodeRequest{ID: "a", Name: "alpha", CPU: 2000, Memory: 2048}); err != nil {
		t.Fatal(err)
	}
	if _, err := cp.RegisterNode(RegisterNodeRequest{ID: "b", Name: "beta", CPU: 2000, Memory: 2048}); err != nil {
		t.Fatal(err)
	}
	first, err := cp.CreateWorkload(CreateWorkloadRequest{Name: "api", Image: "nginx:alpine", CPU: 500, Memory: 256})
	if err != nil {
		t.Fatal(err)
	}
	second, err := cp.CreateWorkload(CreateWorkloadRequest{Name: "worker", Image: "alpine:3.20", CPU: 500, Memory: 256})
	if err != nil {
		t.Fatal(err)
	}
	if first.NodeID == second.NodeID {
		t.Fatalf("expected scheduler to spread equal workloads, got both on %s", first.NodeID)
	}
}

func TestRejectsWhenCapacityIsExhausted(t *testing.T) {
	cp, _ := New("")
	_, _ = cp.RegisterNode(RegisterNodeRequest{ID: "a", Name: "alpha", CPU: 500, Memory: 256})
	_, err := cp.CreateWorkload(CreateWorkloadRequest{Name: "huge", Image: "busybox", CPU: 1000, Memory: 512})
	if !errors.Is(err, ErrNoCapacity) {
		t.Fatalf("expected ErrNoCapacity, got %v", err)
	}
}

func TestDeleteReleasesCapacity(t *testing.T) {
	cp, _ := New("")
	_, _ = cp.RegisterNode(RegisterNodeRequest{ID: "a", Name: "alpha", CPU: 1000, Memory: 1024})
	w, err := cp.CreateWorkload(CreateWorkloadRequest{Name: "job", Image: "busybox", CPU: 750, Memory: 512})
	if err != nil {
		t.Fatal(err)
	}
	if err := cp.DeleteWorkload(w.ID); err != nil {
		t.Fatal(err)
	}
	nodes := cp.ListNodes()
	if nodes[0].UsedCPU != 0 || nodes[0].UsedMemory != 0 {
		t.Fatalf("capacity not released: %+v", nodes[0])
	}
}

func TestNodeCanFetchOnlyItsAssignedWorkloads(t *testing.T) {
	cp, _ := New("")
	_, _ = cp.RegisterNode(RegisterNodeRequest{ID: "a", Name: "alpha", CPU: 1000, Memory: 1024})
	w, _ := cp.CreateWorkload(CreateWorkloadRequest{Name: "job", Image: "busybox", CPU: 250, Memory: 128})
	items, err := cp.ListNodeWorkloads("a")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ID != w.ID {
		t.Fatalf("unexpected assignments: %+v", items)
	}
}

func TestTerminalStatusReleasesCapacityAndRejectsWrongNode(t *testing.T) {
	cp, _ := New("")
	_, _ = cp.RegisterNode(RegisterNodeRequest{ID: "a", Name: "alpha", CPU: 1000, Memory: 1024})
	_, _ = cp.RegisterNode(RegisterNodeRequest{ID: "b", Name: "beta", CPU: 1000, Memory: 1024})
	w, _ := cp.CreateWorkload(CreateWorkloadRequest{Name: "job", Image: "busybox", CPU: 750, Memory: 512})
	wrongNode := "a"
	if w.NodeID == "a" {
		wrongNode = "b"
	}
	if _, err := cp.UpdateWorkloadStatus(w.ID, UpdateWorkloadStatusRequest{NodeID: wrongNode, Status: "running"}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}
	if _, err := cp.UpdateWorkloadStatus(w.ID, UpdateWorkloadStatusRequest{NodeID: w.NodeID, Status: "succeeded"}); err != nil {
		t.Fatal(err)
	}
	for _, n := range cp.ListNodes() {
		if n.ID == w.NodeID && (n.UsedCPU != 0 || n.UsedMemory != 0) {
			t.Fatalf("terminal workload still consumes capacity: %+v", n)
		}
	}
}
