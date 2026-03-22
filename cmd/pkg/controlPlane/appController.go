package control

import(
	"orchestrator/store"
	"orchestrator/task"
	"orchestrator/types"
	"github.com/google/uuid"
	"fmt"
	// "time"
	"context"
)

type AppController struct{
	Store store.Client
}

func TopologicalSort(services map[string]*types.ServiceSpec, deps map[string][]types.Dependency) ([]string, error) {
	inDegree := make(map[string]int)
	graph := make(map[string][]string)

	// initialize
	for name := range services {
		inDegree[name] = 0
	}

	// build graph
	for from, depList := range deps {
		for _, dep := range depList {
			to := dep.TargetService
			graph[to] = append(graph[to], from)
			inDegree[from]++
		}
	}

	// queue for nodes with 0 in-degree
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

func (a *AppController) ReconcileApp(ctx context.Context,app *types.AppGroup) error {
	// Resolve dependency order
	order, err := TopologicalSort(app.Service, app.Dependencies)
	if err != nil {
		return fmt.Errorf("dependency resolution failed for app %s: %w", app.Name, err)
	}

	// Reconcile each service in dependency order
	for _, serviceName := range order {

		spec, exists := app.Service[serviceName]
		if !exists {
			return fmt.Errorf("service %s not found in app %s", serviceName, app.Name)
		}

		//  Call service-level reconciliation
		if err := a.reconcileService(ctx, app, *spec); err != nil {
			return fmt.Errorf("failed to reconcile service %s in app %s: %w",serviceName,app.Name,err,)
		}
	}

	return nil
}

//converting service to tasks
func (a *AppController) reconcileService(ctx context.Context,app *types.AppGroup,spec types.ServiceSpec,) error {
	existingTasks, err := a.Store.ListTasksByService(ctx, app.Name, spec.Name)
	if err != nil {
		return err
	}
	current := len(existingTasks)
	desired := spec.Replicas
	// SCALE UP
	if current < desired {
		toCreate := desired - current
		for i := 0; i < toCreate; i++ {
			t := &task.Task{
				Name :       spec.Name,
				Image:       spec.Image,
				ID:          uuid.New(),
				CPU:         spec.CPU,
				Memory:      spec.Memory,
				Disk:        spec.Disk,
				State:       task.Pending,
				AppName:     app.Name,
				ServiceName: spec.Name,
				// StartTime:   time.Now(),
			}

			if err := a.Store.CreateTask(ctx, t, ""); err != nil {
				return err
			}
		}
	}

	// SCALE DOWN
	// if current > desired {
	// 	toDelete := current - desired

	// 	sort.Slice(existingTasks, func(i, j int) bool {
	// 		return existingTasks[i].State < existingTasks[j].State
	// 	})

	// 	for i := 0; i < toDelete; i++ {
	// 		t := existingTasks[i]
	// 		t.State = task.Completed
	// 		if err := a.store.UpdateTask(ctx, &t); err != nil {
	// 			return err
	// 		}
	// 	}
	// }

	return nil
}

func (a *AppController) CreateApp(ctx context.Context,app *types.AppGroup) error {
	// Basic validation
	if app.Name == "" {
		return fmt.Errorf("app name cannot be empty")
	}
	if len(app.Service) == 0 {
		return fmt.Errorf("at least one service required")
	}
	//Validate dependency references
	for from, deps := range app.Dependencies {
		if _, ok := app.Service[from]; !ok {
			return fmt.Errorf("dependency source service %s not found", from)
		}

		for _, dep := range deps {
			if _, ok := app.Service[dep.TargetService]; !ok {
				return fmt.Errorf("dependency target service %s not found", dep.TargetService)
			}
		}
	}
	// Initialize metadata
	app.Version = 1
	app.Status = types.AppActive
	// Store full desired state
	if err := a.Store.SaveApp(ctx, app); err != nil {
		return err
	}
	// Reconcile → create tasks in topo order
	if err := a.ReconcileApp(ctx, app); err != nil {
		return err
	}
	return nil
}

func (a *AppController) AddDependency(ctx context.Context,appName, from, to string,weight int) error {
	app, err := a.Store.GetApp(ctx, appName)
	if err != nil {
		return err
	}
	dep := types.Dependency{
		TargetService: to,
		MaxNetworkCost: weight,
	}
	app.Dependencies[from] = append(app.Dependencies[from],dep,)
	app.Version++
	return a.Store.SaveApp(ctx, app)
}

func (a *AppController) AddService(ctx context.Context,appName string,serviceName string,replicas int,) error {
	app, err := a.Store.GetApp(ctx, appName)
	if err != nil {
		return err
	}
	app.Service[serviceName] = &types.ServiceSpec{
		Name:     serviceName,
		Replicas: replicas,
	}
	return a.Store.SaveApp(ctx, app)
}

//Removing a service
func (a *AppController) RemoveService(ctx context.Context,appName string,serviceName string,) error {
	return a.Store.DeleteService(ctx, appName, serviceName)
}

//Deleting application
// func (a *AppController) DeleteApp(
// 	ctx context.Context,
// 	appName string,
// ) error {
// 	return a.Store.DeleteApp(ctx, appName)
// }