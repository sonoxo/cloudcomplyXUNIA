package main

import "testing"

func TestNormalizeImage(t *testing.T) {
	cases := map[string]string{
		"nginx:alpine":                   "docker.io/library/nginx:alpine",
		"library/nginx:alpine":           "docker.io/library/nginx:alpine",
		"ghcr.io/owner/app:v1":           "ghcr.io/owner/app:v1",
		"quay.io/org/service:v2":         "quay.io/org/service:v2",
		"localhost:5000/nxyz/app:latest": "localhost:5000/nxyz/app:latest",
	}
	for in, want := range cases {
		got, err := normalizeImage(in)
		if err != nil {
			t.Fatalf("normalize %q: %v", in, err)
		}
		if got != want {
			t.Fatalf("normalize %q = %q, want %q", in, got, want)
		}
	}
}

func TestNormalizeImageRejectsUnsafeInput(t *testing.T) {
	for _, in := range []string{"", "https://example.com/image", "-privileged", "nginx latest"} {
		if _, err := normalizeImage(in); err == nil {
			t.Fatalf("expected %q to fail", in)
		}
	}
}

func TestContainerNameSanitizes(t *testing.T) {
	if got := containerName("w/123"); got != "nxyz-w-123" {
		t.Fatalf("unexpected name %q", got)
	}
}
