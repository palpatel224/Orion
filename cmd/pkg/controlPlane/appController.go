package control

import(
	"orchestrator/node"
	"orchestrator/store"
	"github.com/google/uuid"
)

const(
	AppDraft AppStatus = "Draft"
	AppActive AppStatus ="Active"
)

type AppController struct{
	store store.Client
}

type Dependency struct {
	TargetService string
	MaxNetworkCost int
}

type ServiceSpec struct {
	Name     string
	Image    string
	CPU      int64
	Memory   int64
	Disk     int64
	Replicas int
}

type AppGroup struct {
	Name string
	Version int
	Service map[string]*ServiceSpec
	Dependencies map[string][]Dependency
	Status AppStatus
}

func TopologicalSort(services map[string]*ServiceSpec, deps map[string][]Dependency) ([]string, error) {
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

func (a *AppController) reconcileApp(ctx context.Context,app *AppGroup,) error {
	// Resolve dependency order
	order, err := TopologicalSort(app.Services, app.Dependencies)
	if err != nil {
		return fmt.Errorf("dependency resolution failed for app %s: %w", app.Name, err)
	}

	// Reconcile each service in dependency order
	for _, serviceName := range order {

		spec, exists := app.Services[serviceName]
		if !exists {
			return fmt.Errorf("service %s not found in app %s", serviceName, app.Name)
		}

		//  Call service-level reconciliation
		if err := a.reconcileService(ctx, app, spec); err != nil {
			return fmt.Errorf("failed to reconcile service %s in app %s: %w",serviceName,app.Name,err,)
		}
	}

	return nil
}

//converting service to tasks
func (a *AppController) reconcileService(ctx context.Context,app *AppGroup,spec ServiceSpec,) error {
	existingTasks, err := a.store.ListTasks(ctx, app.Name, spec.Name)
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
				ID:          uuid.New(),
				AppName:     app.Name,
				ServiceName: spec.Name,
				CPU:         spec.CPU,
				Memory:      spec.Memory,
				Disk:        spec.Disk,
				State:       task.Pending,
			}

			if err := a.store.CreateTask(ctx, t, ""); err != nil {
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

func (a *AppController) CreateApp(ctx context.Context,app *AppGroup,) error {
	// Basic validation
	if app.Name == "" {
		return fmt.Errorf("app name cannot be empty")
	}
	if len(app.Services) == 0 {
		return fmt.Errorf("at least one service required")
	}
	//Validate dependency references
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
	//  Detect cycles using Topological Sort
	order, err := TopologicalSort(app.Services, app.Dependencies)
	if err != nil {
		return fmt.Errorf("invalid dependency graph: %w", err)
	}
	// Initialize metadata
	app.Version = 1
	app.Status = AppActive
	// Store full desired state
	if err := a.store.SaveApp(ctx, app); err != nil {
		return err
	}
	// Reconcile → create tasks in topo order
	if err := a.reconcileApp(ctx, app, order); err != nil {
		return err
	}
	return nil
}

func(a *AppController) AddDependency(ctx context.Context,appName,from,to string,weight int) error{
	app,err:=a.store.GetApp(ctx,appName)
	if err!=nil{
		return err
	}
	if app.Dependencies[from]==nil{
		app.Dependencies[from]=make(map[string]int);
	}
	app.Dependencies[from][to]=weight;
	app.Version++;
	return a.store.SaveApp(ctx,app);
}

func (a *AppController) AddService(ctx context.Context,appName string,serviceName string,replicas int,) error {
	app, err := a.store.GetApp(ctx, appName)
	if err != nil {
		return err
	}
	app.Services[serviceName] = &Service{
		Name:     serviceName,
		Replicas: replicas,
	}
	return a.store.SaveApp(ctx, app)
}

//Removing a service
func (a *AppController) RemoveService(ctx context.Context,appName string,serviceName string,) error {
	return a.store.DeleteService(ctx, appName, serviceName)
}

//Deleting application
// func (a *AppController) DeleteApp(
// 	ctx context.Context,
// 	appName string,
// ) error {
// 	return a.store.DeleteApp(ctx, appName)
// }