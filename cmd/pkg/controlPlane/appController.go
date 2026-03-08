package control

import (
	"context"
	"fmt"
)

type AppStatus string

const (
	AppDraft  AppStatus = "Draft"
	AppActive AppStatus = "Active"
)

type Dependency struct {
	TargetService  string `json:"target_service"`
	MaxNetworkCost int    `json:"max_network_cost"`
}

type ServiceSpec struct {
	Name     string `json:"name"`
	Image    string `json:"image"`
	CPU      int64  `json:"cpu"`
	Memory   int64  `json:"memory"`
	Disk     int64  `json:"disk"`
	Replicas int    `json:"replicas"`
}

type AppGroup struct {
	Name         string                  `json:"name"`
	Version      int                     `json:"version"`
	Services     map[string]*ServiceSpec `json:"services"`
	Dependencies map[string][]Dependency `json:"dependencies"`
	Status       AppStatus               `json:"status"`
}

type AppStore interface {
	SaveApp(ctx context.Context, app *AppGroup) error
	GetApp(ctx context.Context, name string) (*AppGroup, error)
	ListApp(ctx context.Context) ([]*AppGroup, error)
	DeleteService(ctx context.Context, appName string, serviceName string) error
}

type AppController struct {
	store AppStore
}

func NewAppController(store AppStore) *AppController {
	if store == nil {
		return nil
	}
	return &AppController{store: store}
}

func TopologicalSort(services map[string]*ServiceSpec, deps map[string][]Dependency) ([]string, error) {
	inDegree := make(map[string]int)
	graph := make(map[string][]string)

	for name := range services {
		inDegree[name] = 0
	}

	for from, depList := range deps {
		for _, dep := range depList {
			to := dep.TargetService
			graph[to] = append(graph[to], from)
			inDegree[from]++
		}
	}

	queue := []string{}
	for svc, degree := range inDegree {
		if degree == 0 {
			queue = append(queue, svc)
		}
	}

	var order []string
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		order = append(order, current)

		for _, neighbor := range graph[current] {
			inDegree[neighbor]--
			if inDegree[neighbor] == 0 {
				queue = append(queue, neighbor)
			}
		}
	}

	if len(order) != len(services) {
		return nil, fmt.Errorf("cycle detected in dependencies")
	}

	return order, nil
}

func (a *AppController) ReconcileApp(ctx context.Context, app *AppGroup) error {
	_ = ctx
	if app == nil {
		return fmt.Errorf("app cannot be nil")
	}

	if len(app.Services) == 0 {
		return nil
	}

	if _, err := TopologicalSort(app.Services, app.Dependencies); err != nil {
		return fmt.Errorf("dependency resolution failed for app %s: %w", app.Name, err)
	}

	return nil
}

func (a *AppController) ReconcileAll(ctx context.Context) error {
	if a == nil || a.store == nil {
		return nil
	}

	apps, err := a.store.ListApp(ctx)
	if err != nil {
		return err
	}

	for _, app := range apps {
		if app == nil {
			continue
		}
		if err := a.ReconcileApp(ctx, app); err != nil {
			return err
		}
	}

	return nil
}

func (a *AppController) CreateApp(ctx context.Context, app *AppGroup) error {
	if a == nil || a.store == nil {
		return fmt.Errorf("app store is not configured")
	}
	if app == nil {
		return fmt.Errorf("app cannot be nil")
	}
	if app.Name == "" {
		return fmt.Errorf("app name cannot be empty")
	}
	if len(app.Services) == 0 {
		return fmt.Errorf("at least one service required")
	}
	if app.Dependencies == nil {
		app.Dependencies = map[string][]Dependency{}
	}

	for from, deps := range app.Dependencies {
		if _, ok := app.Services[from]; !ok {
			return fmt.Errorf("dependency source service %s not found", from)
		}
		for _, dep := range deps {
			if _, ok := app.Services[dep.TargetService]; !ok {
				return fmt.Errorf("dependency target service %s not found", dep.TargetService)
			}
		}
	}

	if err := a.ReconcileApp(ctx, app); err != nil {
		return err
	}

	if app.Version == 0 {
		app.Version = 1
	}
	if app.Status == "" {
		app.Status = AppActive
	}

	return a.store.SaveApp(ctx, app)
}

func (a *AppController) AddDependency(ctx context.Context, appName, from, to string, weight int) error {
	if a == nil || a.store == nil {
		return fmt.Errorf("app store is not configured")
	}

	app, err := a.store.GetApp(ctx, appName)
	if err != nil {
		return err
	}

	if app.Dependencies == nil {
		app.Dependencies = map[string][]Dependency{}
	}

	deps := app.Dependencies[from]
	updated := false
	for idx := range deps {
		if deps[idx].TargetService == to {
			deps[idx].MaxNetworkCost = weight
			updated = true
			break
		}
	}
	if !updated {
		deps = append(deps, Dependency{TargetService: to, MaxNetworkCost: weight})
	}
	app.Dependencies[from] = deps
	app.Version++

	return a.store.SaveApp(ctx, app)
}

func (a *AppController) AddService(ctx context.Context, appName string, serviceName string, replicas int) error {
	if a == nil || a.store == nil {
		return fmt.Errorf("app store is not configured")
	}

	app, err := a.store.GetApp(ctx, appName)
	if err != nil {
		return err
	}

	if app.Services == nil {
		app.Services = map[string]*ServiceSpec{}
	}
	app.Services[serviceName] = &ServiceSpec{Name: serviceName, Replicas: replicas}
	app.Version++

	return a.store.SaveApp(ctx, app)
}

func (a *AppController) RemoveService(ctx context.Context, appName string, serviceName string) error {
	if a == nil || a.store == nil {
		return fmt.Errorf("app store is not configured")
	}
	return a.store.DeleteService(ctx, appName, serviceName)
}
