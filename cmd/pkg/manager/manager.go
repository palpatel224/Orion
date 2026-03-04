package manager

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"orchestrator/node"
	"orchestrator/scheduler"
	"orchestrator/controlPlane"
	"orchestrator/store"
	"orchestrator/task"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/docker/go-connections/nat"
	"github.com/golang-collections/collections/queue"
	"github.com/google/uuid"
	"go.etcd.io/etcd/api/v3/mvccpb"
	clientv3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/client/v3/concurrency"
)

type ManagerRole string

const heartbeatStaleAfter = 30 * time.Second
const defaultLeaderTTL = 15 * time.Second

const (
	ManagerRoleLeader   ManagerRole = "leader"
	ManagerRoleFollower ManagerRole = "follower"
)

type Manager struct {
	ID             string
	etcdStore      *store.Client
	Store          store.Store
	leaderKey      string
	managerKey     string
	role           ManagerRole
	taskWatchStop  context.CancelFunc
	scheduler      scheduler.Scheduler
	roleMu         sync.RWMutex
	electionStop   chan struct{}
	WorkerClient   WorkerCommunicator
	etcdSession    *concurrency.Session
	AdvertiseAddr  string
	initialWorkers []string
	Pending        queue.Queue
	AppController  controlPlane.AppController
}

type Config struct {
	Workers       []string
	ID            string
	SchedulerType string
	Store         store.Store
	Role          ManagerRole
	AdvertiseAddr string
	WorkerClient  WorkerCommunicator
}

type Elector struct {
	Session  *concurrency.Session
	Election *concurrency.Election
	ID       string
}

type managerInfo struct {
	ID        string    `json:"id"`
	Address   string    `json:"address,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}

func (m *Manager) reconcileAllApps(ctx context.Context) {

	apps, err := m.Store.ListApps(ctx)
	if err != nil {
		log.Println("Failed to list apps:", err)
		return
	}

	for _, app := range apps {
		err := m.AppController.reconcileApp(ctx, app)
		if err != nil {
			log.Println("Reconcile failed for app:", app.Name)
		}
	}
}

func (m *Manager) CurrentRole() ManagerRole {
	m.roleMu.RLock()
	defer m.roleMu.RUnlock()
	return m.role
}


func (m *Manager) runAppController(ctx context.Context){
	ticker:=time.NewTicker(10*time.Second)
	defer ticker.Stop()

	for{
		select{
		case <-ctx.Done():
			log.Printf("AppController stopper")
			return
		case<-ticker.C:
			if err:=m.reconcileAllApps(ctx);err!=nil{
				log.Printf("Reconcile error:",err)
			}
		}	
	}
}

func (m *Manager) onBecameLeader(ctx context.Context) {
	wasLeader := m.isLeader()
	m.setRole(ManagerRoleLeader)
	if !wasLeader {
		log.Printf("Manager %s: became leader", m.ID)
		c, cancel := context.WithTimeout(ctx, 5*time.Second)
		m.registerWorkers(c, m.initialWorkers)
		cancel()
		go m.runAppController(leaderCtx)
	}
}

func defaultManagerID() string {
	if envID := os.Getenv("ORION_MANAGER_ID"); envID != "" {
		return envID
	}

	if host, err := os.Hostname(); err == nil && host != "" {
		return host
	}

	return uuid.New().String()
}

func NewWithConfig(cfg Config) *Manager {
	role := cfg.Role
	if role == "" {
		role = ManagerRoleFollower
	}
	id := cfg.ID
	if id == "" {
		id = defaultManagerID()
	}
	advertiseAddr := cfg.AdvertiseAddr
	if advertiseAddr == "" {
		advertiseAddr = os.Getenv("ORION_MANAGER_ADDRESS")
	}
	var s scheduler.Scheduler
	switch cfg.SchedulerType {
	case "roundrobin":
		s = &scheduler.RoundRobin{Name: "roundrobin"}
	// case "epvm":
	// 	s = &scheduler.RoundRobin{Name: "rounfrobin"}
	default:
		s = &scheduler.RoundRobin{Name: "roundrobin"}
	}
	wc := cfg.WorkerClient
	if wc == nil {
		wc = NewHTTPWorkerClient(nil)
	}

	initialWorkers := append([]string(nil), cfg.Workers...)

	m := &Manager{
		ID:             id,
		Pending:        *queue.New(),
		scheduler:      s,
		WorkerClient:   wc,
		Store:          cfg.Store,
		AdvertiseAddr:  advertiseAddr,
		initialWorkers: initialWorkers,
		electionStop:   make(chan struct{}),
	}

	if etcdStore, ok := cfg.Store.(*store.Client); ok {
		m.etcdStore = etcdStore
		m.leaderKey = etcdStore.LeaderKey()
		m.managerKey = etcdStore.ManagerKey(m.ID)
	}

	m.setRole(role)

	if m.etcdStore != nil {
		m.startLeaderElection()
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	m.registerWorkers(ctx, cfg.Workers)

	return m
}

var ErrNotLeader = errors.New("manager: not leader")

func (m *Manager) buildManagerInfo() managerInfo {
	return managerInfo{ID: m.ID, Address: m.AdvertiseAddr, Timestamp: time.Now().UTC()}
}

func (m *Manager) startLeaderElection() {
	go m.leaderElectionLoop()
}

func (m *Manager) watchLeaderKey(ctx context.Context, session *concurrency.Session) {
	if session == nil {
		return
	}
	client := m.etcdStore.Cli
	resp, err := client.Get(ctx, m.leaderKey)
	if err == nil {
		if resp == nil || resp.Count == 0 {
			m.tryAcquireLeadership(session)
		} else if len(resp.Kvs) > 0 {
			info, decodeErr := decodeManagerInfo(resp.Kvs[0].Value)
			if decodeErr != nil {
				log.Printf("Manager %s: failed to decode leader info: %v", m.ID, decodeErr)
			} else if info.ID == m.ID {
				m.setRole(ManagerRoleLeader)
			} else {
				m.setRole(ManagerRoleFollower)
			}
		}
	}
	watchOpts := []clientv3.OpOption{}
	if resp != nil {
		watchOpts = append(watchOpts, clientv3.WithRev(resp.Header.Revision+1))
	}
	watchChan := client.Watch(ctx, m.leaderKey, watchOpts...)
	for watchResp := range watchChan {
		if watchResp.Canceled {
			return
		}

		for _, ev := range watchResp.Events {
			switch ev.Type {
			case mvccpb.DELETE:
				log.Printf("Manager %s: detected missing leader key; attempting promotion", m.ID)
				m.setRole(ManagerRoleFollower)
				m.tryAcquireLeadership(session)
			case mvccpb.PUT:
				info, decodeErr := decodeManagerInfo(ev.Kv.Value)
				if decodeErr != nil {
					log.Printf("Manager %s: failed to decode leader info update: %v", m.ID, decodeErr)
					continue
				}

				if info.ID == m.ID {
					m.setRole(ManagerRoleLeader)
				} else {
					m.setRole(ManagerRoleFollower)
				}
			}
		}
	}
}

func (m *Manager) watchPendingTasks(ctx context.Context) {
	//Check for any pending tasks before starting watch
	m.schedulePendingSnapshot()

	watchChan := m.etcdStore.Cli.Watch(ctx, m.etcdStore.TaskPrefix(), clientv3.WithPrefix())
	for {
		select {
		case <-ctx.Done():
			return
		case watchResp, ok := <-watchChan:
			if !ok || watchResp.Canceled {
				return
			}
			for _, ev := range watchResp.Events {
				if ev.Type != mvccpb.PUT {
					continue
				}
				key := string(ev.Kv.Key)
				if !strings.HasSuffix(key, "/status") {
					continue
				}
				var state task.State
				if err := json.Unmarshal(ev.Kv.Value, &state); err != nil {
					log.Printf("Manager %s: failed to decode task state update for key %s: %v", m.ID, key, err)
					continue
				}
				if state != task.Pending {
					continue
				}
				taskID, err := parseTaskIDFromStateKey(key)
				if err != nil {
					log.Printf("Manager %s: unable to parse task ID from key %s: %v", m.ID, key, err)
					continue
				}

				go m.tryToSchedulePendingTask(taskID)
			}
		}
	}
}

func parseTaskIDFromStateKey(key string) (uuid.UUID, error) {
	segments := strings.Split(strings.Trim(key, "/"), "/")
	for idx, segment := range segments {
		if segment == "tasks" && idx+1 < len(segments) {
			return uuid.Parse(segments[idx+1])
		}
	}
	return uuid.Nil, fmt.Errorf("could not parse task ID from key %s", key)
}

func (m *Manager) setRole(role ManagerRole) {
	m.roleMu.Lock()
	prev := m.role
	m.role = role
	m.roleMu.Unlock()

	if prev == role {
		return
	}
	if role == ManagerRoleLeader {
		m.startTaskWatch()
	} else {
		m.stopTaskWatch()
	}
}

func (m *Manager) startTaskWatch() {
	if m.etcdStore == nil {
		return
	}
	//Always stops any existing watch before starting a new one to avoid
	//duplicate schedulers running after role changes
	m.stopTaskWatch()
	ctx, cancel := context.WithCancel(context.Background())
	m.taskWatchStop = cancel
	go m.watchPendingTasks(ctx)
}

func (m *Manager) stopTaskWatch() {
	if m.taskWatchStop != nil {
		m.taskWatchStop()
		m.taskWatchStop = nil
	}
}

func (m *Manager) isLeader() bool {
	m.roleMu.RLock()
	role := m.role
	m.roleMu.RUnlock()
	if role == ManagerRoleLeader {
		return true
	}
	return false
}

// Actually checks which tasks are pending
func (m *Manager) schedulePendingSnapshot() {
	if !m.isLeader() || m.etcdStore == nil || m.Store == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	record, err := m.Store.ListTasks(ctx)
	if err != nil {
		log.Printf("Manager %v Unable to list tasks for pending snapshots %v\n", m.ID, err)
		return
	}
	for _, rec := range record {
		if rec.Task != nil && rec.Task.State == task.Pending {
			go m.tryToSchedulePendingTask(rec.Task.ID)
		}
	}
}

func (m *Manager) tryToSchedulePendingTask(taskID uuid.UUID) {
	if taskID == uuid.Nil {
		return
	}
	if m.Store == nil || m.etcdStore == nil || !m.isLeader() {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)

	resp, _, err := m.Store.GetTask(ctx, taskID)
	cancel()
	if err != nil {
		log.Printf("Error in getting task %v from the etcd %v\n", taskID, err)
		return
	}
	if resp.State != task.Pending {
		return
	}
	workerctx, workerCancel := context.WithTimeout(context.Background(), 5*time.Second)
	worker, err := m.SelectWorker(workerctx, *resp)
	workerCancel()
	if err != nil {
		log.Printf("Manager %s : no worker is selected for task %s : %v\n", m.ID, resp.ID, err)
		return
	}
	//Now update it in the etcd
	assignctx, assignCancel := context.WithTimeout(context.Background(), 5*time.Second)
	succeeded, e := m.etcdStore.AssignPendingTasks(assignctx, resp, worker.ID)
	assignCancel()
	if e != nil {
		log.Printf("Manager %s: failed to assign task %s to worker %s: %v", m.ID, resp.ID, worker.ID, err)
		return
	}
	if !succeeded {
		log.Printf("Manager %s: task %s was already scheduled by another manager", m.ID, resp.ID)
		return
	}
	resp.State = task.Scheduled
	m.dispatchTaskToWorker(*resp, worker)
}

func (m *Manager) dispatchTaskToWorker(t task.Task, worker *store.Worker) {
	if worker == nil {
		return
	}
	// send task to the worker
	//if there is some error in sending task to worker or in scheduling
	//then it has to be rescheduled and in the etcd changes are to be chamgeed back

	te := task.TaskEvent{
		ID:        uuid.New(),
		State:     task.Scheduled,
		Timestamp: time.Now().UTC(),
		Task:      t,
	}
	_, errResp, err := m.WorkerClient.StartTask(worker.Address, te)
	if err != nil {
		log.Printf("Manager %s: dispatch to worker %s for task %s failed: %v", m.ID, worker.ID, t.ID, err)
		m.resetTaskToPending(t)
		return
	}

	if errResp != nil {
		log.Printf("Manager %s: worker %s rejected task %s: %s", m.ID, worker.ID, t.ID, errResp.Message)
		m.resetTaskToPending(t)
		return
	}

	// Mark task as running after the worker acknowledged receipt.
	t.State = task.Running
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	if err := m.Store.UpdateTaskState(ctx, &t, worker.ID); err != nil {
		log.Printf("Manager %s: failed to mark task %s running: %v", m.ID, t.ID, err)
	}
	cancel()
}

func (m *Manager) resetTaskToPending(t task.Task) {
	if m.Store == nil {
		return
	}
	t.State = task.Pending
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	err := m.Store.UpdateTaskState(ctx, &t, "")
	defer cancel()
	if err != nil {
		log.Printf("Manager %s: failed to revert task %s to pending: %v", m.ID, t.ID, err)
	}
}

func (m *Manager) registerManager(session *concurrency.Session) error {
	if session == nil {
		return fmt.Errorf("etcd session is not initialized")
	}

	info := m.buildManagerInfo()
	payload, err := json.Marshal(info)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)

	_, err = m.etcdStore.Cli.Put(ctx, m.managerKey, string(payload), clientv3.WithLease(session.Lease()))
	cancel()
	return err
}

func (m *Manager) activeWorkers(ctx context.Context) ([]store.Worker, error) {
	if m.Store == nil {
		return nil, errors.New("store not configured")
	}
	workers, err := m.Store.ListWorkers(ctx)
	if err != nil {
		return nil, err
	}
	cutoff := time.Now().UTC().Add(-heartbeatStaleAfter)
	live := make([]store.Worker, 0, len(workers))
	for _, w := range workers {
		if w.Heartbeat.After(cutoff) {
			live = append(live, w)
		}
	}

	if len(live) == 0 {
		return nil, fmt.Errorf("no workers with heartbeat in the last %s", heartbeatStaleAfter)
	}

	return live, nil
}

func decodeManagerInfo(payload []byte) (*managerInfo, error) {
	if len(payload) == 0 {
		return nil, fmt.Errorf("empty manager info payload")
	}

	var info managerInfo
	if err := json.Unmarshal(payload, &info); err != nil {
		return nil, err
	}

	return &info, nil
}

func (m *Manager) leaderElectionLoop() {
	retryDelay := 2 * time.Second

	for {
		select {
		case <-m.electionStop:
			return
		default:
		}

		if m.etcdStore == nil {
			return
		}

		session, err := concurrency.NewSession(m.etcdStore.Cli, concurrency.WithTTL(int(defaultLeaderTTL/time.Second)))
		if err != nil {
			log.Printf("Manager %s: unable to create etcd session for leader election: %v", m.ID, err)
			time.Sleep(retryDelay)
			continue
		}

		m.etcdSession = session

		if err := m.registerManager(session); err != nil {
			log.Printf("Manager %s: failed to register manager presence: %v", m.ID, err)
			_ = session.Close()
			time.Sleep(retryDelay)
			continue
		}
		// Attempt to become the leader immediately on startup.
		m.tryAcquireLeadership(session)

		watchCtx, cancel := context.WithCancel(context.Background())
		go m.watchLeaderKey(watchCtx, session)

		<-session.Done()
		cancel()
		m.setRole(ManagerRoleFollower)
		log.Printf("Manager %s: etcd session closed; stepping down to follower", m.ID)
		time.Sleep(retryDelay)
	}
}

func (m *Manager) tryAcquireLeadership(session *concurrency.Session) bool {
	if session == nil {
		return false
	}

	info := m.buildManagerInfo()
	payload, err := json.Marshal(info)
	if err != nil {
		log.Printf("Manager %s: failed to marshal leader payload: %v", m.ID, err)
		return false
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	txn := m.etcdStore.Cli.Txn(ctx).If(
		clientv3.Compare(clientv3.CreateRevision(m.leaderKey), "=", 0),
	).Then(
		clientv3.OpPut(m.leaderKey, string(payload), clientv3.WithLease(session.Lease())),
	)

	resp, err := txn.Commit()
	if err != nil {
		log.Printf("Manager %s: leader election transaction failed: %v", m.ID, err)
		return false
	}
	fmt.Printf("Not failed \n")
	if resp.Succeeded {
		m.onBecameLeader(session.Ctx())
		return true
	}
	m.setRole(ManagerRoleFollower)
	return false
}

func (m *Manager) SelectWorker(ctx context.Context, t task.Task) (*store.Worker, error) {
	if m.Store == nil {
		return nil, errors.New("store not configured ")
	}
	workers, err := m.activeWorkers(ctx)
	if err != nil {
		return nil, err
	}
	workerMap := make(map[string]store.Worker)
	nodes := make([]*node.Node, 0, len(workers))
	for _, w := range workers {
		workerMap[w.ID] = w
		n := node.NewNode(w.ID, w.Address, "worker")
		nodes = append(nodes, n)
	}
	//now select candidates
	candidates := m.scheduler.SelectCandidateNodes(&t, nodes)
	if len(candidates) == 0 {
		return nil, fmt.Errorf("There is no available candidate that match resource requirement for task\n")
	}
	scores := m.scheduler.Score(&t, nodes)
	selected_candidates := m.scheduler.Pick(scores, candidates)
	if selected_candidates == nil {
		return nil, fmt.Errorf("Scheduler failed to pick a worker ")
	}
	selectedWorker, ok := workerMap[selected_candidates.ID]
	if !ok {
		return nil, fmt.Errorf("Selected worker not found with ID %v\n", selected_candidates.ID)
	}
	return &selectedWorker, nil
}

func (m *Manager) updateTasks() {
	if m.Store == nil {
		return
	}
	if !m.isLeader() {
		log.Printf("Manager is not the leader\n")
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	workers, err := m.activeWorkers(ctx)
	cancel()
	if err != nil {
		log.Printf("Error in getting active workers %v\n", err)
		return
	}
	if len(workers) == 0 {
		fmt.Printf("There is no active worker currently \n")
		return
	}
	for _, w := range workers {
		tasks, er := m.WorkerClient.FetchTasks(w.Address)
		if er != nil {
			log.Printf("Error in fetching tasks from worker %v : %v\n", w.ID, er)
			continue
		}
		for _, t := range tasks {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			stored_task, _, e := m.Store.GetTask(ctx, t.ID)
			cancel()
			if e != nil {
				if errors.Is(e, store.ErrNotFound) {
					log.Printf("Task with this id not found %v : %v\v", t.ID, e)
					continue
				}
				log.Printf("Error retrieving task from etcd %v : %v\n", t.ID, e)
				continue
			}
			stored_task.State = t.State
			stored_task.ContainerId = t.ContainerId
			stored_task.StartTime = t.StartTime
			stored_task.FinishTime = t.FinishTime

			//now update the task in the etcd

			updateContext, updateCancel := context.WithTimeout(context.Background(), 5*time.Second)
			err := m.Store.UpdateTaskState(updateContext, stored_task, w.ID)
			updateCancel()
			if err != nil {
				log.Printf("Error in updating task in etcd %v\n", err)
				continue
			}
		}
	}
}

func (m *Manager) SendWork() {
	if !m.isLeader() {
		log.Printf("Manager is not leader \n")
		return
	}
	if m.Pending.Len() == 0 {
		log.Printf("No work in queue \n")
		return
	}
	e := m.Pending.Dequeue()
	te, ok := e.(task.TaskEvent)
	if !ok {
		log.Printf("Unexpected item in queue: %T", e)
		return
	}
	//task event pulled successfully from the pending queue

	log.Printf("Task Event %v is pulled successfully from queue \n", te.ID)
	if m.Store == nil {
		log.Println("Store not configured; cannot process task")
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	existingTask, existingWorker, existingErr := m.Store.GetTask(ctx, te.Task.ID)
	cancel()
	if existingErr != nil && !errors.Is(existingErr, store.ErrNotFound) {
		log.Printf("Error retrieving task %s from store: %v", te.Task.ID, existingErr)
		m.Pending.Enqueue(te)
		return
	}

	//if the task is already running on a worker
	if existingWorker != "" && existingTask != nil {
		if te.State == task.Completed && task.ValidStateTransition(existingTask.State, te.State) {
			m.StopTask(existingWorker, te.Task.ID.String())
			return
		}
		log.Printf("Invalid request: existing task %s is in state %v and cannot transition to the completed state\n",
			existingTask.ID.String(), existingTask.State)
		return
	}
	//if existing worker is nil

	t := te.Task
	ctx, cancel = context.WithTimeout(context.Background(), 5*time.Second)
	w, e := m.SelectWorker(ctx, t)
	cancel()
	if e != nil {
		log.Printf("Error selecting worker for task %v : %v\n", t.ID, e)
		m.Pending.Enqueue(te)
		return
	}
	//interact with the worker client to start the task on the selected worker
	newTask, errResp, err := m.WorkerClient.StartTask(w.Address, te)
	if err != nil {
		log.Printf("Error connecting to %v: %v\n", w.ID, err)
		m.Pending.Enqueue(te)
		return
	}

	if errResp != nil {
		log.Printf("Response error (%d): %s", errResp.HTTPStatusCode, errResp.Message)
		return
	}

	if newTask != nil {
		ctx, cancel = context.WithTimeout(context.Background(), 5*time.Second)
		if errors.Is(existingErr, store.ErrNotFound) {
			if err := m.Store.CreateTask(ctx, newTask, w.ID); err != nil {
				log.Printf("Error persisting task %s: %v", newTask.ID, err)
			}
		} else {
			if err := m.Store.UpdateTaskState(ctx, newTask, w.ID); err != nil {
				log.Printf("Error updating task %s: %v", newTask.ID, err)
			}
		}
		cancel()
		log.Printf("%#v\n", *newTask)
	}
}

func (m *Manager) LeaderAddress(ctx context.Context) (string, error) {
	if m.isLeader() {
		if m.AdvertiseAddr == "" {
			log.Printf("Leader address not configured \n")
			return "", errors.New("Advertise address not configured for leader")
		}
		return m.AdvertiseAddr, nil
	}
	if m.etcdStore == nil {
		return "", fmt.Errorf("etcd store not configured; cannot resolve leader")
	}

	lookupCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	resp, err := m.etcdStore.Cli.Get(lookupCtx, m.leaderKey)
	if err != nil {
		return "", err
	}

	if resp.Count == 0 || len(resp.Kvs) == 0 {
		return "", fmt.Errorf("no leader present")
	}

	info, err := decodeManagerInfo(resp.Kvs[0].Value)
	if err != nil {
		return "", err
	}

	if info.Address != "" {
		return info.Address, nil
	}

	//query the specific manager key for specific data if present

	managerKey := m.etcdStore.ManagerKey(info.ID)
	managerResp, err := m.etcdStore.Cli.Get(lookupCtx, managerKey)
	if err != nil {
		return "", err
	}

	if managerResp.Count == 0 || len(managerResp.Kvs) == 0 {
		return "", fmt.Errorf("leader metadata missing for %s", info.ID)
	}

	managerInfo, err := decodeManagerInfo(managerResp.Kvs[0].Value)
	if err != nil {
		return "", err
	}

	if managerInfo.Address == "" {
		return "", fmt.Errorf("leader address not published")
	}

	return managerInfo.Address, nil
}

func (m *Manager) registerWorkers(ctx context.Context, worker []string) {
	if m.Store == nil {
		log.Printf("Etcd not configured \n")
		return
	}
	if !m.isLeader() {
		log.Printf("Manager %s is in follower role; skipping worker registration", m.ID)
		return
	}
	for _, w := range worker {
		meta := store.Worker{ID: w, Address: w, Heartbeat: time.Now().UTC()}
		if err := m.Store.RegisterWorker(ctx, meta); err != nil {
			log.Printf("Error registering worker %s: %v", w, err)
		}

	}
}

func (m *Manager) GetTasks() []*task.Task {
	if m.Store == nil {
		log.Printf("Etcd store not intialised \n")
		return []*task.Task{}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	taskRecord, err := m.Store.ListTasks(ctx)
	if err != nil {
		log.Printf("Error retrieving tasks from store: %v", err)
		return []*task.Task{}
	}
	tasks := make([]*task.Task, 0, len(taskRecord))
	for _, rec := range taskRecord {
		tasks = append(tasks, rec.Task)
	}
	return tasks
}

func (m *Manager) UpdateTasks() {
	for {
		if !m.isLeader() {
			log.Printf("Manager %s is in follower role; skipping worker task sync", m.ID)
			time.Sleep(15 * time.Second)
			continue
		}
		log.Println("Checking for task updates from workers")
		m.updateTasks()
		log.Println("Tasks update completed")
		log.Println("Sleeping for 15 seconds")
		time.Sleep(15 * time.Second)
	}
}

func (m *Manager) ProcessTasks() {
	for {
		if !m.isLeader() {
			log.Printf("Manager %s is in follower role; skipping task processing", m.ID)
			time.Sleep(10 * time.Second)
			continue
		}
		log.Println("Processing any tasks in the queue")
		m.SendWork()
		log.Printf("Sleeping for 10 seconds")
		time.Sleep(10 * time.Second)
	}
}

func getHostPort(ports nat.PortMap) *string {
	for k, _ := range ports {
		return &(ports[k][0].HostPort)
	}
	return nil
}

func (m *Manager) checkTaskHealth(t task.Task) error {
	log.Printf("Calling health check for task %s: %s\n", t.ID, t.HealthCheck)
	if m.Store == nil {
		return errors.New("store not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	_, workerID, err := m.Store.GetTask(ctx, t.ID)
	cancel()
	if err != nil {
		msg := fmt.Sprintf("Error retrieving worker for task %s: %v", t.ID, err)
		log.Println(msg)
		return errors.New(msg)
	}

	if workerID == "" {
		msg := fmt.Sprintf("No worker assigned for task %s", t.ID)
		log.Println(msg)
		return errors.New(msg)
	}

	hostPort := getHostPort(t.PortBindings)
	if hostPort == nil {
		msg := fmt.Sprintf("No host port found for task %s", t.ID)
		log.Println(msg)
		return errors.New(msg)
	}

	worker := strings.Split(workerID, ":")
	url := fmt.Sprintf("http://%s:%s%s", worker[0], *hostPort, t.HealthCheck)

	log.Printf("Calling health check for task %s: %s\n", t.ID, url)

	resp, err := http.Get(url)

	if err != nil {
		msg := fmt.Sprintf("Error connecting to health check %s", url)
		log.Println(msg)
		return errors.New(msg)
	}

	if resp.StatusCode != http.StatusOK {
		msg := fmt.Sprintf("Error health check for task %s did not return 200\n", t.ID)
		log.Println(msg)
		return errors.New(msg)
	}

	log.Printf("Task %s health check response: %v\n", t.ID, resp.StatusCode)

	return nil

}

func (m *Manager) doHealthChecks() {
	for _, t := range m.GetTasks() {
		if t.State == task.Running && t.RestartCount < 3 {
			if t.HealthCheck != "" {
				err := m.checkTaskHealth(*t)
				if err != nil {
					if t.RestartCount < 3 {
						m.RestartTask(t)
					}
				}
			}
		} else if t.State == task.Failed && t.RestartCount < 3 {
			m.RestartTask(t)
		}
	}
}

//task is in running state ---> checkTaskHealth
//task is in failed state --->attempt to restart the task
//if the task's health check fails -->try to restart the task
//rest all states like pending ,scheduled or completed there is no need to check health

func (m *Manager) RestartTask(t *task.Task) {
	if !m.isLeader() {
		log.Printf("Manager %s is in follower role; skipping restart for task %s", m.ID, t.ID)
		return
	}
	if m.Store == nil {
		log.Println("Store not configured; cannot restart task")
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	_, workerID, err := m.Store.GetTask(ctx, t.ID)
	cancel()
	if err != nil {
		log.Printf("Error fetching task %s for restart: %v", t.ID, err)
		return
	}

	if workerID == "" {
		log.Printf("No worker assignment found for task %s; cannot restart", t.ID)
		return
	}

	workerCtx, workerCancel := context.WithTimeout(context.Background(), 5*time.Second)
	workers, err := m.activeWorkers(workerCtx)
	workerCancel()
	if err != nil {
		log.Printf("Error listing workers for restart of task %s: %v", t.ID, err)
		return
	}
	var assignedWorker *store.Worker

	for _, w := range workers {
		if w.ID == workerID {
			copy := w
			assignedWorker = &copy
			break
		}
	}

	if assignedWorker == nil {
		log.Printf("Assigned worker %s for task %s not found among active workers", workerID, t.ID)
		m.resetTaskToPending(*t)
		return
	}

	t.State = task.Scheduled
	t.RestartCount++

	ctx, cancel = context.WithTimeout(context.Background(), 5*time.Second)
	if err := m.Store.UpdateTaskState(ctx, t, workerID); err != nil {
		log.Printf("Error persisting restart state for task %s: %v", t.ID, err)
	}
	cancel()

	m.dispatchTaskToWorker(*t, assignedWorker)
}

func (m *Manager) DoHealthChecks() {
	for {
		if !m.isLeader() {
			log.Printf("Manager %s is in follower role; skipping health checks", m.ID)
			time.Sleep(60 * time.Second)
			continue
		}
		log.Println("Performing task health check")
		m.doHealthChecks()
		log.Println("Task health checks completed")
		log.Println("Sleeping for 60 seconds")
		time.Sleep(60 * time.Second)
	}
}

func (m *Manager) StopTask(worker string, taskID string) {
	if !m.isLeader() {
		log.Printf("Manager %s is in follower role; skipping stopping tasks", m.ID)
		return
	}
	err := m.WorkerClient.StopTask(worker, taskID)
	if err != nil {
		log.Printf("Error sending request: %v\n", err)
		return
	}
	if id, err := uuid.Parse(taskID); err == nil {
		if m.Store == nil {
			log.Println("Store not configured; cannot persist stop")
			return
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		t, _, err := m.Store.GetTask(ctx, id)
		cancel()
		if err != nil {
			log.Printf("Error fetching task %s to persist stop: %v", taskID, err)
			return
		}

		t.State = task.Completed
		t.FinishTime = time.Now().UTC()

		ctx, cancel = context.WithTimeout(context.Background(), 5*time.Second)
		if err := m.Store.UpdateTaskState(ctx, t, worker); err != nil {
			log.Printf("Error persisting stop for task %s: %v", taskID, err)
		}
		cancel()
	}

	log.Printf("Task %s has been scheduled to be stopped", taskID)
}

func (m *Manager) AddTask(te task.TaskEvent) error {
	if !m.isLeader() {
		log.Print("Manager is a follower;Adding task event is skipped")
		return errors.New("Manager is a follower;Adding task event is skipped")
	}
	if m.Store == nil {
		log.Printf("Etcd store not configured")
		return errors.New("Etcd store not configured")
	}
	if te.State == task.Completed {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, workerID, err := m.Store.GetTask(ctx, te.Task.ID)
		if err != nil {
			return err
		}

		if workerID == "" {
			return fmt.Errorf("no worker assignment found for task %s", te.Task.ID)
		}

		m.StopTask(workerID, te.Task.ID.String())
		return nil
	}
	if te.Task.ID == uuid.Nil {
		te.Task.ID = uuid.New()
	}
	te.Task.State = task.Pending
	te.Timestamp = time.Now().UTC()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Persist immediately so a manager restart can restore tasks even before dispatch.
	return m.Store.CreateTask(ctx, &te.Task, "")
}
