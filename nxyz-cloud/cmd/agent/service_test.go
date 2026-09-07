package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestProbeHealth(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	client := &http.Client{Timeout: time.Second}
	if got := probeHealth(client, srv.URL, "/health"); got != "healthy" {
		t.Fatalf("probeHealth = %q, want healthy", got)
	}
	if got := probeHealth(client, srv.URL, "/missing"); got != "unhealthy" {
		t.Fatalf("probeHealth missing = %q, want unhealthy", got)
	}
	if got := probeHealth(client, srv.URL, ""); got != "unknown" {
		t.Fatalf("probeHealth without path = %q, want unknown", got)
	}
}
