package main

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/sonoxo/nxyz-cloud/internal/cloud"
)

//go:embed index.html
var dashboard embed.FS

type api struct{ cp *cloud.ControlPlane }

func main() {
	addr := env("NXYZ_LISTEN", ":8080")
	state := env("NXYZ_STATE", "/data/state.json")
	cp, err := cloud.New(state)
	if err != nil {
		log.Fatalf("initialize NXYZ Cloud: %v", err)
	}

	a := &api{cp: cp}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", a.health)
	mux.HandleFunc("GET /api/v1/system", a.system)
	mux.HandleFunc("GET /api/v1/nodes", a.nodes)
	mux.HandleFunc("POST /api/v1/nodes/register", a.registerNode)
	mux.HandleFunc("POST /api/v1/nodes/{id}/heartbeat", a.heartbeat)
	mux.HandleFunc("GET /api/v1/workloads", a.workloads)
	mux.HandleFunc("POST /api/v1/workloads", a.createWorkload)
	mux.HandleFunc("DELETE /api/v1/workloads/{id}", a.deleteWorkload)
	mux.HandleFunc("GET /metrics", a.metrics)
	mux.HandleFunc("GET /", a.dashboard)

	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			cp.ReapStaleNodes(20 * time.Second)
		}
	}()

	srv := &http.Server{Addr: addr, Handler: requestLog(mux), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second}
	log.Printf("☁️  NXYZ Cloud control plane listening on %s", addr)
	if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}

func (a *api) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, 200, map[string]any{"status": "ok", "service": "nxyz-controlplane"})
}
func (a *api) system(w http.ResponseWriter, _ *http.Request) { writeJSON(w, 200, a.cp.Summary()) }
func (a *api) nodes(w http.ResponseWriter, _ *http.Request)  { writeJSON(w, 200, a.cp.ListNodes()) }
func (a *api) workloads(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, 200, a.cp.ListWorkloads())
}

func (a *api) registerNode(w http.ResponseWriter, r *http.Request) {
	var req cloud.RegisterNodeRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, 400, err)
		return
	}
	n, err := a.cp.RegisterNode(req)
	if err != nil {
		writeError(w, 400, err)
		return
	}
	writeJSON(w, 201, n)
}

func (a *api) heartbeat(w http.ResponseWriter, r *http.Request) {
	n, err := a.cp.HeartbeatNode(r.PathValue("id"))
	if errors.Is(err, cloud.ErrNotFound) {
		writeError(w, 404, err)
		return
	}
	if err != nil {
		writeError(w, 500, err)
		return
	}
	writeJSON(w, 200, n)
}

func (a *api) createWorkload(w http.ResponseWriter, r *http.Request) {
	var req cloud.CreateWorkloadRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, 400, err)
		return
	}
	item, err := a.cp.CreateWorkload(req)
	if errors.Is(err, cloud.ErrNoCapacity) {
		writeError(w, 409, err)
		return
	}
	if err != nil {
		writeError(w, 400, err)
		return
	}
	writeJSON(w, 201, item)
}

func (a *api) deleteWorkload(w http.ResponseWriter, r *http.Request) {
	err := a.cp.DeleteWorkload(r.PathValue("id"))
	if errors.Is(err, cloud.ErrNotFound) {
		writeError(w, 404, err)
		return
	}
	if err != nil {
		writeError(w, 500, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *api) metrics(w http.ResponseWriter, _ *http.Request) {
	s := a.cp.Summary()
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	fmt.Fprintf(w, "# HELP nxyz_nodes_total Number of registered nodes.\n# TYPE nxyz_nodes_total gauge\nnxyz_nodes_total %d\n", s.Nodes)
	fmt.Fprintf(w, "# HELP nxyz_nodes_healthy Number of healthy nodes.\n# TYPE nxyz_nodes_healthy gauge\nnxyz_nodes_healthy %d\n", s.HealthyNodes)
	fmt.Fprintf(w, "# HELP nxyz_workloads_total Number of scheduled workloads.\n# TYPE nxyz_workloads_total gauge\nnxyz_workloads_total %d\n", s.Workloads)
	fmt.Fprintf(w, "# HELP nxyz_cpu_allocated_millicores Allocated CPU.\n# TYPE nxyz_cpu_allocated_millicores gauge\nnxyz_cpu_allocated_millicores %d\n", s.AllocatedCPU)
	fmt.Fprintf(w, "# HELP nxyz_memory_allocated_mb Allocated memory.\n# TYPE nxyz_memory_allocated_mb gauge\nnxyz_memory_allocated_mb %d\n", s.AllocatedMemory)
}

func (a *api) dashboard(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	b, err := dashboard.ReadFile("index.html")
	if err != nil {
		writeError(w, 500, err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(b)
}

func decodeJSON(r *http.Request, dst any) error {
	defer r.Body.Close()
	d := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 1<<20))
	d.DisallowUnknownFields()
	if err := d.Decode(dst); err != nil {
		return fmt.Errorf("invalid json: %w", err)
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}
func env(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}
func requestLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(start).Round(time.Millisecond))
	})
}
