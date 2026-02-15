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
}