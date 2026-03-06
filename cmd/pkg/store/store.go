package store

import(
	"time"
	"context"
	"errors"
	"orchestrator/task"
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
	SaveApp(ctx context.Context,app *controlPlane.AppGroup)error
	GetApp(ctx context.Context,name string)(*controlPlane.AppGroup,error)
	ListApp(ctx context.Context)([]*controlPlane.AppGroup,error)
	DeleteService(ctx context.Context, appName string, serviceName string) error
}