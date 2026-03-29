package manifest

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	yaml "gopkg.in/yaml.v2"
)

type Kind string

const (
	KindTask Kind = "task"
	KindApp  Kind = "app"
)

type taskSubmission struct {
	ID        string   `json:"ID,omitempty" yaml:"id,omitempty"`
	State     string   `json:"State,omitempty" yaml:"state,omitempty"`
	Timestamp string   `json:"Timestamp,omitempty" yaml:"timestamp,omitempty"`
	Task      taskSpec `json:"Task" yaml:"task"`
}

type taskSpec struct {
	Name          string   `json:"Name" yaml:"name"`
	Image         string   `json:"Image" yaml:"image"`
	CPU           int64    `json:"CPU" yaml:"cpu"`
	Memory        int64    `json:"Memory" yaml:"memory"`
	Disk          int64    `json:"Disk" yaml:"disk"`
	RestartPolicy string   `json:"RestartPolicy,omitempty" yaml:"restartPolicy,omitempty"`
	Cmd           []string `json:"Cmd,omitempty" yaml:"cmd,omitempty"`
}

type taskManifest struct {
	APIVersion string `yaml:"apiVersion"`
	Kind       string `yaml:"kind"`
	Metadata   struct {
		Name string `yaml:"name"`
	} `yaml:"metadata"`
	Spec taskSpec `yaml:"spec"`
}

type appSubmission struct {
	Name         string                     `json:"Name" yaml:"name"`
	Service      map[string]*appServiceSpec `json:"Service" yaml:"service"`
	Dependencies map[string][]appDependency `json:"Dependencies,omitempty" yaml:"dependencies,omitempty"`
}

type appServiceSpec struct {
	Name     string `json:"Name" yaml:"name"`
	Image    string `json:"Image" yaml:"image"`
	CPU      int64  `json:"CPU" yaml:"cpu"`
	Memory   int64  `json:"Memory" yaml:"memory"`
	Disk     int64  `json:"Disk" yaml:"disk"`
	Replicas int    `json:"Replicas" yaml:"replicas"`
}

type appDependency struct {
	TargetService  string `json:"TargetService" yaml:"targetService"`
	MaxNetworkCost int    `json:"MaxNetworkCost" yaml:"maxNetworkCost"`
}

type appManifest struct {
	APIVersion string `yaml:"apiVersion"`
	Kind       string `yaml:"kind"`
	Metadata   struct {
		Name string `yaml:"name"`
	} `yaml:"metadata"`
	Spec struct {
		Services     map[string]*appServiceSpec `yaml:"services"`
		Dependencies map[string][]appDependency `yaml:"dependencies,omitempty"`
	} `yaml:"spec"`
}

func ParseSubmissionFile(filePath string) ([]byte, Kind, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, "", err
	}

	ext := strings.ToLower(filepath.Ext(filePath))
	switch ext {
	case ".yaml", ".yml":
		return parseYAMLSubmission(data)
	case ".json":
		return parseJSONSubmission(data)
	default:
		return nil, "", fmt.Errorf("unsupported file extension %q: use .json, .yaml, or .yml", ext)
	}
}

func ParseTaskSubmissionFile(filePath string) ([]byte, error) {
	out, kind, err := ParseSubmissionFile(filePath)
	if err != nil {
		return nil, err
	}
	if kind != KindTask {
		return nil, fmt.Errorf("manifest kind %q is not supported by this command path", kind)
	}
	return out, nil
}

func parseJSONSubmission(data []byte) ([]byte, Kind, error) {
	var envelope struct {
		Task         json.RawMessage `json:"Task"`
		Service      json.RawMessage `json:"Service"`
		Dependencies json.RawMessage `json:"Dependencies"`
	}

	if err := strictDecodeJSON(data, &envelope); err != nil {
		return nil, "", fmt.Errorf("invalid manifest JSON: %w", err)
	}

	if len(envelope.Task) > 0 {
		payload, err := parseJSONTask(data)
		if err != nil {
			return nil, "", err
		}
		if err := validateTaskSubmission(&payload); err != nil {
			return nil, "", err
		}
		out, err := json.Marshal(payload)
		if err != nil {
			return nil, "", fmt.Errorf("failed to encode task payload: %w", err)
		}
		return out, KindTask, nil
	}

	if len(envelope.Service) > 0 {
		payload, err := parseJSONApp(data)
		if err != nil {
			return nil, "", err
		}
		if err := validateAppSubmission(&payload); err != nil {
			return nil, "", err
		}
		out, err := json.Marshal(payload)
		if err != nil {
			return nil, "", fmt.Errorf("failed to encode app payload: %w", err)
		}
		return out, KindApp, nil
	}

	return nil, "", fmt.Errorf("unable to infer manifest kind from JSON: expected Task or Service field")
}

func parseYAMLSubmission(data []byte) ([]byte, Kind, error) {
	var kindProbe struct {
		Kind string `yaml:"kind"`
	}
	if err := yaml.UnmarshalStrict(data, &kindProbe); err == nil && strings.TrimSpace(kindProbe.Kind) != "" {
		switch {
		case strings.EqualFold(kindProbe.Kind, "Task"):
			payload, err := parseYAMLTask(data)
			if err != nil {
				return nil, "", err
			}
			if err := validateTaskSubmission(&payload); err != nil {
				return nil, "", err
			}
			out, err := json.Marshal(payload)
			if err != nil {
				return nil, "", fmt.Errorf("failed to encode task payload: %w", err)
			}
			return out, KindTask, nil
		case strings.EqualFold(kindProbe.Kind, "App"):
			payload, err := parseYAMLApp(data)
			if err != nil {
				return nil, "", err
			}
			if err := validateAppSubmission(&payload); err != nil {
				return nil, "", err
			}
			out, err := json.Marshal(payload)
			if err != nil {
				return nil, "", fmt.Errorf("failed to encode app payload: %w", err)
			}
			return out, KindApp, nil
		default:
			return nil, "", fmt.Errorf("invalid YAML kind %q: supported kinds are Task or App", kindProbe.Kind)
		}
	}

	payload, taskErr := parseYAMLTask(data)
	if taskErr == nil {
		if err := validateTaskSubmission(&payload); err != nil {
			return nil, "", err
		}
		out, err := json.Marshal(payload)
		if err != nil {
			return nil, "", fmt.Errorf("failed to encode task payload: %w", err)
		}
		return out, KindTask, nil
	}

	appPayload, appErr := parseYAMLApp(data)
	if appErr == nil {
		if err := validateAppSubmission(&appPayload); err != nil {
			return nil, "", err
		}
		out, err := json.Marshal(appPayload)
		if err != nil {
			return nil, "", fmt.Errorf("failed to encode app payload: %w", err)
		}
		return out, KindApp, nil
	}

	return nil, "", fmt.Errorf("invalid YAML manifest: task parse error: %v; app parse error: %v", taskErr, appErr)
}

func parseJSONTask(data []byte) (taskSubmission, error) {
	var payload taskSubmission
	if err := strictDecodeJSON(data, &payload); err != nil {
		return taskSubmission{}, fmt.Errorf("invalid task JSON: %w", err)
	}
	return payload, nil
}

func parseJSONApp(data []byte) (appSubmission, error) {
	var payload appSubmission
	if err := strictDecodeJSON(data, &payload); err != nil {
		return appSubmission{}, fmt.Errorf("invalid app JSON: %w", err)
	}
	return payload, nil
}

func parseYAMLTask(data []byte) (taskSubmission, error) {
	var manifest taskManifest
	if err := yaml.UnmarshalStrict(data, &manifest); err == nil {
		if manifest.Kind != "" && !strings.EqualFold(manifest.Kind, "Task") {
			return taskSubmission{}, fmt.Errorf("invalid task YAML: kind must be Task, got %q", manifest.Kind)
		}

		name := strings.TrimSpace(manifest.Metadata.Name)
		if name != "" && strings.TrimSpace(manifest.Spec.Name) == "" {
			manifest.Spec.Name = name
		}

		return taskSubmission{Task: manifest.Spec}, nil
	}

	var payload taskSubmission
	if err := yaml.UnmarshalStrict(data, &payload); err != nil {
		return taskSubmission{}, fmt.Errorf("invalid task YAML: %w", err)
	}

	return payload, nil
}

func parseYAMLApp(data []byte) (appSubmission, error) {
	var manifest appManifest
	if err := yaml.UnmarshalStrict(data, &manifest); err == nil {
		if manifest.Kind != "" && !strings.EqualFold(manifest.Kind, "App") {
			return appSubmission{}, fmt.Errorf("invalid app YAML: kind must be App, got %q", manifest.Kind)
		}

		name := strings.TrimSpace(manifest.Metadata.Name)
		if name == "" {
			return appSubmission{}, fmt.Errorf("invalid app YAML: metadata.name is required")
		}

		payload := appSubmission{
			Name:         name,
			Service:      manifest.Spec.Services,
			Dependencies: manifest.Spec.Dependencies,
		}
		return payload, nil
	}

	var payload appSubmission
	if err := yaml.UnmarshalStrict(data, &payload); err != nil {
		return appSubmission{}, fmt.Errorf("invalid app YAML: %w", err)
	}

	return payload, nil
}

func validateTaskSubmission(payload *taskSubmission) error {
	name := strings.TrimSpace(payload.Task.Name)
	image := strings.TrimSpace(payload.Task.Image)

	if name == "" {
		return fmt.Errorf("task name is required")
	}
	if image == "" {
		return fmt.Errorf("task image is required")
	}
	if payload.Task.CPU <= 0 {
		return fmt.Errorf("task cpu must be > 0")
	}
	if payload.Task.Memory <= 0 {
		return fmt.Errorf("task memory must be > 0")
	}
	if payload.Task.Disk <= 0 {
		return fmt.Errorf("task disk must be > 0")
	}

	payload.Task.Name = name
	payload.Task.Image = image
	return nil
}

func validateAppSubmission(payload *appSubmission) error {
	payload.Name = strings.TrimSpace(payload.Name)
	if payload.Name == "" {
		return fmt.Errorf("app name is required")
	}
	if len(payload.Service) == 0 {
		return fmt.Errorf("at least one app service is required")
	}

	for svcKey, spec := range payload.Service {
		if spec == nil {
			return fmt.Errorf("service %q cannot be null", svcKey)
		}
		if strings.TrimSpace(spec.Name) == "" {
			spec.Name = svcKey
		}
		spec.Name = strings.TrimSpace(spec.Name)
		spec.Image = strings.TrimSpace(spec.Image)

		if spec.Name == "" {
			return fmt.Errorf("service %q must have a name", svcKey)
		}
		if spec.Image == "" {
			return fmt.Errorf("service %q image is required", svcKey)
		}
		if spec.CPU <= 0 {
			return fmt.Errorf("service %q cpu must be > 0", svcKey)
		}
		if spec.Memory <= 0 {
			return fmt.Errorf("service %q memory must be > 0", svcKey)
		}
		if spec.Disk <= 0 {
			return fmt.Errorf("service %q disk must be > 0", svcKey)
		}
		if spec.Replicas <= 0 {
			return fmt.Errorf("service %q replicas must be > 0", svcKey)
		}
	}

	for from, deps := range payload.Dependencies {
		if _, ok := payload.Service[from]; !ok {
			return fmt.Errorf("dependency source service %q not found", from)
		}
		for _, dep := range deps {
			if strings.TrimSpace(dep.TargetService) == "" {
				return fmt.Errorf("dependency target for service %q cannot be empty", from)
			}
			if _, ok := payload.Service[dep.TargetService]; !ok {
				return fmt.Errorf("dependency target service %q not found", dep.TargetService)
			}
		}
	}

	return nil
}

func strictDecodeJSON(data []byte, out any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(out); err != nil {
		return err
	}

	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return fmt.Errorf("multiple JSON values in one file")
		}
		return err
	}

	return nil
}
