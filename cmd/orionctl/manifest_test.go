package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func writeTempManifest(t *testing.T, ext, content string) string {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, "task"+ext)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write test manifest: %v", err)
	}
	return path
}

func TestParseTaskSubmissionFileYAMLManifest(t *testing.T) {
	path := writeTempManifest(t, ".yaml", `apiVersion: orion/v1
kind: Task
metadata:
  name: frontend
spec:
  image: nginx:latest
  cpu: 1
  memory: 128
  disk: 2
  cmd:
    - nginx
    - -g
    - daemon off;
`)

	payload, err := parseTaskSubmissionFile(path)
	if err != nil {
		t.Fatalf("expected YAML manifest to parse, got error: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("expected valid JSON payload, got error: %v", err)
	}

	taskObj, ok := decoded["Task"].(map[string]any)
	if !ok {
		t.Fatalf("expected Task object in payload")
	}

	if taskObj["Name"] != "frontend" {
		t.Fatalf("expected task name frontend, got %v", taskObj["Name"])
	}
}

func TestParseTaskSubmissionFileYAMLUnknownField(t *testing.T) {
	path := writeTempManifest(t, ".yaml", `apiVersion: orion/v1
kind: Task
metadata:
  name: frontend
spec:
  image: nginx:latest
  cpu: 1
  memory: 128
  disk: 2
  memroy: 99
`)

	_, err := parseTaskSubmissionFile(path)
	if err == nil {
		t.Fatal("expected strict YAML parse to fail for unknown field")
	}
}

func TestParseTaskSubmissionFileJSONMissingRequired(t *testing.T) {
	path := writeTempManifest(t, ".json", `{
  "Task": {
    "Name": "frontend",
    "CPU": 1,
    "Memory": 128,
    "Disk": 2
  }
}`)

	_, err := parseTaskSubmissionFile(path)
	if err == nil {
		t.Fatal("expected validation error for missing image")
	}
}

func TestParseTaskSubmissionFileJSONTypeMismatch(t *testing.T) {
	path := writeTempManifest(t, ".json", `{
  "Task": {
    "Name": "frontend",
    "Image": "nginx:latest",
    "CPU": "one",
    "Memory": 128,
    "Disk": 2
  }
}`)

	_, err := parseTaskSubmissionFile(path)
	if err == nil {
		t.Fatal("expected decode failure for type mismatch")
	}
}

func TestParseSubmissionFileAppYAML(t *testing.T) {
	path := writeTempManifest(t, ".yaml", `apiVersion: orion/v1
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
      replicas: 2
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
`)

	payload, kind, err := parseSubmissionFile(path)
	if err != nil {
		t.Fatalf("expected app YAML to parse, got error: %v", err)
	}
	if kind != submissionKindApp {
		t.Fatalf("expected kind %q, got %q", submissionKindApp, kind)
	}

	var decoded map[string]any
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("expected valid JSON payload, got error: %v", err)
	}

	if decoded["Name"] != "shop" {
		t.Fatalf("expected app name shop, got %v", decoded["Name"])
	}
}

func TestParseSubmissionFileAppYAMLInvalidDependency(t *testing.T) {
	path := writeTempManifest(t, ".yaml", `apiVersion: orion/v1
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
  dependencies:
    frontend:
      - targetService: backend
        maxNetworkCost: 3
`)

	_, _, err := parseSubmissionFile(path)
	if err == nil {
		t.Fatal("expected validation error for missing dependency target service")
	}
}
