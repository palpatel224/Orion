package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/aditip149209/Orion/pkg/cli"
)

func TestAppYAMLToManagerEndpointIntegration(t *testing.T) {
	var gotPath string
	var gotMethod string
	var gotAppName string
	var gotServices int

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMethod = r.Method

		var body struct {
			Name    string         `json:"Name"`
			Service map[string]any `json:"Service"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		gotAppName = body.Name
		gotServices = len(body.Service)
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "app.yaml")
	manifest := `apiVersion: orion/v1
kind: App
metadata:
  name: shop
spec:
  services:
    frontend:
      image: nginx:latest
      cpu: 1
      memory: 128
      disk: 2
      replicas: 1
    api:
      image: myorg/api:v1
      cpu: 1
      memory: 256
      disk: 4
      replicas: 1
  dependencies:
    frontend:
      - targetService: api
        maxNetworkCost: 5
`
	if err := os.WriteFile(manifestPath, []byte(manifest), 0o644); err != nil {
		t.Fatalf("failed to write app manifest: %v", err)
	}

	payload, kind, err := parseSubmissionFile(manifestPath)
	if err != nil {
		t.Fatalf("parseSubmissionFile returned error: %v", err)
	}
	if kind != submissionKindApp {
		t.Fatalf("expected kind %q, got %q", submissionKindApp, kind)
	}

	client := cli.NewClient(srv.URL)
	if err := client.SubmitApp(payload); err != nil {
		t.Fatalf("SubmitApp returned error: %v", err)
	}

	if gotMethod != http.MethodPost {
		t.Fatalf("expected method %s, got %s", http.MethodPost, gotMethod)
	}
	if gotPath != "/app" {
		t.Fatalf("expected path /app, got %s", gotPath)
	}
	if gotAppName != "shop" {
		t.Fatalf("expected app name shop, got %s", gotAppName)
	}
	if gotServices != 2 {
		t.Fatalf("expected 2 services, got %d", gotServices)
	}
}
