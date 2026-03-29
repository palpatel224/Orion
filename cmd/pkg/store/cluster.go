package store

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"orchestrator/task"
	"orchestrator/types"
	"strings"
	"time"

	"github.com/google/uuid"
	clientv3 "go.etcd.io/etcd/client/v3"
)

type Client struct {
	Cli *clientv3.Client
}

type TaskRecord struct {
	Task     *task.Task
	WorkerID string
}

// creating etcd client
func NewEtcdStore(endpoints []string) (*Client, error) {
	cli, err := clientv3.New(clientv3.Config{
		Endpoints:   endpoints,
		DialTimeout: time.Second * 5,
	})
	if err != nil {
		log.Printf("Error in creating client %v\n", err)
		return nil, err
	}
	return &Client{Cli: cli}, nil
}

func (c *Client) servicePrefix(appName string) string {
	return fmt.Sprintf("/apps/%s", appName)
}

func (c *Client) appPrefix() string {
	return "/apps"
}

func (c *Client) taskPrefix() string {
	return "/tasks/"
}
func (c *Client) TaskPrefix() string {
	return c.taskPrefix()
}

func (c *Client) taskStateKey(id uuid.UUID) string {
	return fmt.Sprintf("/tasks/%s/state", id)
}

func (c *Client) taskWorkerKey(id uuid.UUID) string {
	return fmt.Sprintf("/tasks/%s/worker", id)
}

func (c *Client) taskKey(id uuid.UUID) string {
	return fmt.Sprintf("/tasks/%s", id)
}

func (c *Client) workerHeartbeatKey(id string) string {
	return fmt.Sprintf("/workers/%s/heartbeat", id)
}

func (c *Client) LeaderKey() string {
	return "/managers/leader"
}

func (c *Client) managerKey(id string) string {
	return fmt.Sprintf("/manager/%s", id)
}

func (c *Client) ManagerKey(id string) string {
	return c.managerKey(id)
}

func (c *Client) workerKey(id string) string {
	return fmt.Sprintf("/worker/%s", id)
}

func (c *Client) GetApp(ctx context.Context, name string) (*types.AppGroup, error) {
	key := c.servicePrefix(name)
	resp, err := c.Cli.Get(ctx, key)
	if err != nil {
		return nil, err
	}
	if len(resp.Kvs) == 0 {
		return nil, fmt.Errorf("app not found")
	}

	var app types.AppGroup
	if err := json.Unmarshal(resp.Kvs[0].Value, &app); err != nil {
		return nil, err
	}
	return &app, nil
}

func (c *Client) ListApp(ctx context.Context) ([]*types.AppGroup, error) {
	prefix := c.appPrefix()
	resp, err := c.Cli.Get(ctx, prefix, clientv3.WithPrefix())
	if err != nil {
		return nil, err
	}

	apps := make([]*types.AppGroup, 0, len(resp.Kvs))
	for _, kv := range resp.Kvs {
		var app types.AppGroup
		if err := json.Unmarshal(kv.Value, &app); err != nil {
			return nil, err
		}
		apps = append(apps, &app)
	}

	return apps, nil
}

func (c *Client) SaveApp(ctx context.Context, app *types.AppGroup) error {
	if app == nil {
		return fmt.Errorf("app cannot be nil")
	}

	key := c.servicePrefix(app.Name)
	data, err := json.Marshal(app)
	if err != nil {
		return err
	}

	resp, err := c.Cli.Get(ctx, key)
	if err != nil {
		return err
	}

	if len(resp.Kvs) == 0 {
		_, err = c.Cli.Put(ctx, key, string(data))
		return err
	}

	txnResp, err := c.Cli.Txn(ctx).
		If(clientv3.Compare(clientv3.ModRevision(key), "=", resp.Kvs[0].ModRevision)).
		Then(clientv3.OpPut(key, string(data))).
		Commit()
	if err != nil {
		return err
	}
	if !txnResp.Succeeded {
		return fmt.Errorf("conflict: app modified by someone else")
	}

	return nil
}

func (c *Client) DeleteService(ctx context.Context, appName string, serviceName string) error {
	key := fmt.Sprintf("/apps/%s", appName)

	// 1. Get the data AND the Revision
	resp, err := c.Cli.Get(ctx, key)
	if err != nil {
		return err
	}
	if len(resp.Kvs) == 0 {
		return fmt.Errorf("app not found")
	}

	// Save the revision for the transaction check
	rev := resp.Kvs[0].ModRevision

	var app types.AppGroup
	if err := json.Unmarshal(resp.Kvs[0].Value, &app); err != nil {
		return err
	}

	if _, exists := app.Service[serviceName]; !exists {
		return fmt.Errorf("service not found")
	}

	// 2. Modify
	delete(app.Service, serviceName)
	updated, err := json.Marshal(app)
	if err != nil {
		return err
	}

	// 3. Execute Transaction
	// "If the key's revision hasn't changed, Put the update. Otherwise, fail."
	txnResp, err := c.Cli.Txn(ctx).
		If(clientv3.Compare(clientv3.ModRevision(key), "=", rev)).
		Then(clientv3.OpPut(key, string(updated))).
		Commit()

	if err != nil {
		return err
	}
	if !txnResp.Succeeded {
		return fmt.Errorf("conflict detected: app was modified by another process")
	}

	return nil
}

func (c *Client) AddTaskEvent(key string, t task.TaskEvent) uint32 {
	data, err := json.Marshal(t)
	if err != nil {
		log.Printf("Error in marshaling task event %v \n", err)
		return 1
	}
	k := fmt.Sprintf("/taskDb/%s", t.ID.String())
	log.Printf("task id is %v\n", k)
	ctx := context.Background()
	resp, er := c.Cli.Put(ctx, k, string(data))
	if er != nil {
		log.Printf("Error in adding key-value pair to etcd %v\n", er)
		return 1
	}
	// this is the cluster id which sent the response
	fmt.Printf("Cluster ID is %v\n", resp.Header.ClusterId)
	// this is the member id of the member which sent the response
	fmt.Printf("Member ID is %v\n", resp.Header.MemberId)
	return 0
}

// AddTask stores a task under /tasks/<taskID>
// AddTaskState under /tasks/<taskID>/state
// pending queue

func (c *Client) CreateTask(ctx context.Context, t *task.Task, workerID string) error {
	if t == nil {
		return fmt.Errorf("task cannot be nil")
	}

	taskBytes, err := json.Marshal(t)
	if err != nil {
		return err
	}

	stateBytes, err := json.Marshal(t.State)
	if err != nil {
		return err
	}

	op := []clientv3.Op{
		clientv3.OpPut(c.taskKey(t.ID), string(taskBytes)),
		clientv3.OpPut(c.taskStateKey(t.ID), string(stateBytes)),
	}

	if workerID != "" {
		workerBytes, err := json.Marshal(workerID)
		if err != nil {
			return err
		}
		op = append(op, clientv3.OpPut(c.taskWorkerKey(t.ID), string(workerBytes)))
	}

	_, err = c.Cli.Txn(ctx).Then(op...).Commit()
	return err
}

func (c *Client) GetTask(ctx context.Context, taskID uuid.UUID) (*task.Task, string, error) {
	// key := fmt.Sprintf("/tasks/%s", taskID)
	resp, err := c.Cli.Get(ctx, c.taskKey(taskID))
	if err != nil {
		return nil, "", err
	}
	if len(resp.Kvs) == 0 {
		return nil, "", ErrNotFound
	}
	var t task.Task
	if e := json.Unmarshal(resp.Kvs[0].Value, &t); e != nil {
		return nil, "", e
	}
	stateResp, er := c.Cli.Get(ctx, c.taskStateKey(taskID))
	if er != nil {
		return nil, "", er
	}
	if stateResp.Count > 0 {
		var s task.State
		if err := json.Unmarshal(stateResp.Kvs[0].Value, &s); err != nil {
			return nil, "", err
		}
		t.State = s
	}
	workerResp, err := c.Cli.Get(ctx, c.taskWorkerKey(taskID))
	if err != nil {
		return nil, "", err
	}

	workerID := ""
	if workerResp.Count > 0 {
		if err := json.Unmarshal(workerResp.Kvs[0].Value, &workerID); err != nil {
			return nil, "", err
		}
	}

	return &t, workerID, nil
}

func (c *Client) UpdateTaskState(ctx context.Context, t *task.Task, workerID string) error {
	if t == nil {
		return fmt.Errorf("task cannot be nil")
	}

	_, existingWorker, err := c.GetTask(ctx, t.ID)
	if err != nil {
		return err
	}

	updated := *t

	taskBytes, err := json.Marshal(&updated)
	if err != nil {
		return err
	}

	stateBytes, err := json.Marshal(&updated.State)
	if err != nil {
		return err
	}

	resolvedWorker := existingWorker
	if workerID != "" {
		resolvedWorker = workerID
	}
	op := []clientv3.Op{
		clientv3.OpPut(c.taskKey(updated.ID), string(taskBytes)),
		clientv3.OpPut(c.taskStateKey(updated.ID), string(stateBytes)),
	}

	if resolvedWorker != "" {
		workerBytes, err := json.Marshal(resolvedWorker)
		if err != nil {
			return err
		}
		op = append(op, clientv3.OpPut(c.taskWorkerKey(updated.ID), string(workerBytes)))
	}

	_, err = c.Cli.Txn(ctx).Then(op...).Commit()
	return err
}

// ListTasks returns all tasks and their worker assignments.
func (c *Client) ListTasks(ctx context.Context) ([]TaskRecord, error) {
	pre := "/tasks/"
	resp, err := c.Cli.Get(ctx, pre, clientv3.WithPrefix())
	if err != nil {
		return nil, err
	}

	type taskMeta struct {
		task   *task.Task
		state  *task.State
		worker string
	}

	records := make(map[string]*taskMeta)

	for _, kv := range resp.Kvs {
		segments := strings.Split(strings.TrimPrefix(string(kv.Key), "/"), "/")
		if len(segments) < 2 || segments[0] != "tasks" {
			continue
		}

		id := segments[1]
		meta, ok := records[id]
		if !ok {
			meta = &taskMeta{}
			records[id] = meta
		}

		switch len(segments) {
		case 2:
			var t task.Task
			if err := json.Unmarshal(kv.Value, &t); err != nil {
				return nil, err
			}
			meta.task = &t
		case 3, 4:
			switch segments[2] {
			case "state":
				var s task.State
				if err := json.Unmarshal(kv.Value, &s); err != nil {
					return nil, err
				}
				meta.state = &s
			case "worker":
				var w string
				if err := json.Unmarshal(kv.Value, &w); err != nil {
					return nil, err
				}
				meta.worker = w
			}
		}
	}

	results := make([]TaskRecord, 0, len(records))
	for _, meta := range records {
		if meta.task == nil {
			continue
		}
		if meta.state != nil {
			meta.task.State = *meta.state
		}
		results = append(results, TaskRecord{Task: meta.task, WorkerID: meta.worker})
	}

	return results, nil
}

func (c *Client) ListWorkers(ctx context.Context) ([]Worker, error) {
	key := "/worker"
	resp, err := c.Cli.Get(ctx, key, clientv3.WithPrefix())
	if err != nil {
		return nil, err
	}
	type workerMeta struct {
		worker    *Worker
		heartbeat *time.Time
	}

	records := make(map[string]*workerMeta)

	for _, kv := range resp.Kvs {
		segments := strings.Split(strings.TrimPrefix(string(kv.Key), "/"), "/")
		if len(segments) < 2 || (segments[0] != "workers" && segments[0] != "worker") {
			continue
		}
		id := segments[1]
		meta, ok := records[id]
		if !ok {
			meta = &workerMeta{}
			records[id] = meta
		}
		switch len(segments) {
		case 2:
			var w Worker
			if err := json.Unmarshal(kv.Value, &w); err != nil {
				return nil, err
			}
			meta.worker = &w
		case 3:
			if segments[2] == "heartbeat" {
				var ts time.Time
				if err := json.Unmarshal(kv.Value, &ts); err != nil {
					return nil, err
				}
				meta.heartbeat = &ts
			}
		}
	}

	workers := make([]Worker, 0, len(records))
	for _, meta := range records {
		if meta.worker == nil {
			continue
		}
		if meta.heartbeat != nil {
			meta.worker.Heartbeat = *meta.heartbeat
		}
		workers = append(workers, *meta.worker)
	}

	return workers, nil
}

func (c *Client) UpdateWorkerHeartbeat(ctx context.Context, workerID string, heartbeat time.Time) error {
	// Get existing worker data
	workerKey := c.workerKey(workerID)
	resp, err := c.Cli.Get(ctx, workerKey)
	if err != nil {
		return err
	}
	if resp.Count == 0 {
		return ErrNotFound
	}

	// Parse existing worker data
	var worker Worker
	if err := json.Unmarshal(resp.Kvs[0].Value, &worker); err != nil {
		return err
	}

	// Update heartbeat
	worker.Heartbeat = heartbeat

	// Save updated worker data
	workerBytes, err := json.Marshal(worker)
	if err != nil {
		return err
	}

	// Store heartbeat separately for quick access
	hbBytes, err := json.Marshal(heartbeat)
	if err != nil {
		return err
	}

	_, err = c.Cli.Txn(ctx).Then(
		clientv3.OpPut(workerKey, string(workerBytes)),
		clientv3.OpPut(c.workerHeartbeatKey(workerID), string(hbBytes)),
	).Commit()
	return err
}

// UpdateWorkerStats updates both heartbeat and resource stats for a worker
func (c *Client) UpdateWorkerStats(ctx context.Context, workerID string, heartbeat time.Time, cpuUsage float64, memAvail, memTotal, diskFree, diskTotal uint64) error {
	// Get existing worker data
	workerKey := c.workerKey(workerID)
	resp, err := c.Cli.Get(ctx, workerKey)
	if err != nil {
		return err
	}
	if resp.Count == 0 {
		return ErrNotFound
	}

	// Parse existing worker data
	var worker Worker
	if err := json.Unmarshal(resp.Kvs[0].Value, &worker); err != nil {
		return err
	}

	// Update heartbeat and stats
	worker.Heartbeat = heartbeat
	worker.CPUUsage = cpuUsage
	worker.MemoryAvailable = memAvail
	worker.MemoryTotal = memTotal
	worker.DiskFree = diskFree
	worker.DiskTotal = diskTotal

	// Save updated worker data
	workerBytes, err := json.Marshal(worker)
	if err != nil {
		return err
	}

	// Store heartbeat separately for quick access
	hbBytes, err := json.Marshal(heartbeat)
	if err != nil {
		return err
	}

	_, err = c.Cli.Txn(ctx).Then(
		clientv3.OpPut(workerKey, string(workerBytes)),
		clientv3.OpPut(c.workerHeartbeatKey(workerID), string(hbBytes)),
	).Commit()
	return err
}

func (c *Client) AssignPendingTasks(ctx context.Context, t *task.Task, workerID string) (bool, error) {
	if t == nil {
		return false, fmt.Errorf("task cannot be nil")
	}
	pendingBytes, err := json.Marshal(task.Pending)
	if err != nil {
		return false, err
	}
	scheduledBytes, err := json.Marshal(task.Scheduled)
	if err != nil {
		return false, err
	}
	updated := *t
	updated.State = task.Scheduled

	taskBytes, err := json.Marshal(&updated)
	if err != nil {
		return false, err
	}

	workerBytes, err := json.Marshal(workerID)
	if err != nil {
		return false, err
	}

	txn := c.Cli.Txn(ctx).If(
		clientv3.Compare(clientv3.Value(c.taskStateKey(t.ID)), "=", string(pendingBytes)),
	).Then(
		clientv3.OpPut(c.taskKey(t.ID), string(taskBytes)),
		clientv3.OpPut(c.taskStateKey(t.ID), string(scheduledBytes)),
		clientv3.OpPut(c.taskWorkerKey(t.ID), string(workerBytes)),
	)

	resp, err := txn.Commit()
	if err != nil {
		return false, err
	}

	return resp.Succeeded, nil
}

func (c *Client) RegisterWorker(ctx context.Context, worker Worker) error {
	workerBytes, err := json.Marshal(worker)
	if err != nil {
		return err
	}

	heartbeatBytes, err := json.Marshal(worker.Heartbeat)
	if err != nil {
		return err
	}

	_, err = c.Cli.Txn(ctx).Then(
		clientv3.OpPut(c.workerKey(worker.ID), string(workerBytes)),
		clientv3.OpPut(c.workerHeartbeatKey(worker.ID), string(heartbeatBytes)),
	).Commit()
	return err
}
