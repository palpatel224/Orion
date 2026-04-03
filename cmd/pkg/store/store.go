package store

import(
	"time"
	"context"
	"errors"
	"orchestrator/task"
	"orchestrator/types"
	"github.com/google/uuid"
)

var ErrNotFound=errors.New("Store : item not found")

type Worker struct{
	ID string
	Address string
	Heartbeat time.Time
}

type Store interface {
	CreateTask(ctx context.Context, t *task.Task, workerID string) error
	GetTask(ctx context.Context, id uuid.UUID) (*task.Task, string, error)
	UpdateTaskState(ctx context.Context, t *task.Task, workerID string) error
	ListTasks(ctx context.Context) ([]TaskRecord, error)
	RegisterWorker(ctx context.Context, worker Worker) error
	ListWorkers(ctx context.Context) ([]Worker, error)
	UpdateWorkerHeartbeat(ctx context.Context, workerID string, heartbeat time.Time) error
	//for applications
	SaveApp(ctx context.Context,app *types.AppGroup)error
	GetApp(ctx context.Context,name string)(*types.AppGroup,error)
	ListApp(ctx context.Context)([]*types.AppGroup,error)
	DeleteService(ctx context.Context, appName string, serviceName string) error
	ListTasksByService(ctx context.Context,appName string,serviceName string)([]TaskRecord,error)
}