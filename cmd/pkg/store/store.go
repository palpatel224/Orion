package store

import (
	"context"
	"errors"
	"orchestrator/task"
	"orchestrator/types"
	"time"

	"github.com/google/uuid"
)

var ErrNotFound = errors.New("Store : item not found")

type Worker struct {
	ID              string
	Address         string
	Heartbeat       time.Time
	CPUUsage        float64
	MemoryAvailable uint64
	MemoryTotal     uint64
	DiskFree        uint64
	DiskTotal       uint64
}

type Store interface {
	CreateTask(ctx context.Context, t *task.Task, workerID string) error
	GetTask(ctx context.Context, id uuid.UUID) (*task.Task, string, error)
	UpdateTaskState(ctx context.Context, t *task.Task, workerID string) error
	ListTasks(ctx context.Context) ([]TaskRecord, error)
	RegisterWorker(ctx context.Context, worker Worker) error
	ListWorkers(ctx context.Context) ([]Worker, error)
	UpdateWorkerHeartbeat(ctx context.Context, workerID string, heartbeat time.Time) error
	SaveApp(ctx context.Context, app *types.AppGroup) error
	GetApp(ctx context.Context, name string) (*types.AppGroup, error)
	ListApp(ctx context.Context) ([]*types.AppGroup, error)
	DeleteService(ctx context.Context, appName string, serviceName string) error
}
