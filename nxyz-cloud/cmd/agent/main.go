package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/sonoxo/nxyz-cloud/internal/cloud"
)

func main() {
	cp := strings.TrimRight(env("NXYZ_CONTROL_PLANE", "http://controlplane:8080"), "/")
	id := env("NXYZ_NODE_ID", hostname())
	req := cloud.RegisterNodeRequest{
		ID:      id,
		Name:    env("NXYZ_NODE_NAME", id),
		Address: env("NXYZ_NODE_ADDRESS", id),
		CPU:     envInt("NXYZ_NODE_CPU", 2000),
		Memory:  envInt("NXYZ_NODE_MEMORY_MB", 2048),
	}

	client := &http.Client{Timeout: 5 * time.Second}
	for {
		if err := register(client, cp, req); err == nil {
			break
		} else {
			log.Printf("register failed: %v", err)
		}
		time.Sleep(2 * time.Second)
	}
	log.Printf("🛰️  NXYZ agent %s registered with %s", id, cp)

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		if err := heartbeat(client, cp, id); err != nil {
			log.Printf("heartbeat failed: %v; re-registering", err)
			_ = register(client, cp, req)
		}
	}
}

func register(client *http.Client, cp string, req cloud.RegisterNodeRequest) error {
	b, _ := json.Marshal(req)
	resp, err := client.Post(cp+"/api/v1/nodes/register", "application/json", bytes.NewReader(b))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		data, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("status %s: %s", resp.Status, strings.TrimSpace(string(data)))
	}
	return nil
}
func heartbeat(client *http.Client, cp, id string) error {
	req, _ := http.NewRequest(http.MethodPost, cp+"/api/v1/nodes/"+id+"/heartbeat", nil)
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("status %s", resp.Status)
	}
	return nil
}
func env(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}
func envInt(key string, fallback int) int {
	v, err := strconv.Atoi(os.Getenv(key))
	if err == nil && v > 0 {
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
