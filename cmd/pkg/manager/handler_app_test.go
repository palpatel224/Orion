package manager

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	control "orchestrator/controlPlane"
	"orchestrator/store"
	"orchestrator/task"
	"orchestrator/types"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

type fakeAppStore struct {
	saveErr error
	saved   *types.AppGroup
}

func (f *fakeAppStore) CreateTask(ctx context.Context, t *task.Task, workerID string) error {
	return nil
}

func (f *fakeAppStore) GetTask(ctx context.Context, id uuid.UUID) (*task.Task, string, error) {
	return nil, "", store.ErrNotFound
}

func (f *fakeAppStore) UpdateTaskState(ctx context.Context, t *task.Task, workerID string) error {
	return nil
}

func (f *fakeAppStore) ListTasks(ctx context.Context) ([]store.TaskRecord, error) {
	return []store.TaskRecord{}, nil
}

func (f *fakeAppStore) RegisterWorker(ctx context.Context, worker store.Worker) error {
	return nil
}

func (f *fakeAppStore) ListWorkers(ctx context.Context) ([]store.Worker, error) {
	return []store.Worker{}, nil
}

func (f *fakeAppStore) UpdateWorkerHeartbeat(ctx context.Context, workerID string, heartbeat time.Time) error {
	return nil
}

func (f *fakeAppStore) SaveApp(ctx context.Context, app *types.AppGroup) error {
	if f.saveErr != nil {
		return f.saveErr
	}
	cp := *app
	f.saved = &cp
	return nil
}

func (f *fakeAppStore) GetApp(ctx context.Context, name string) (*types.AppGroup, error) {
	if f.saved == nil {
		return nil, store.ErrNotFound
	}
	cp := *f.saved
	return &cp, nil
}

func (f *fakeAppStore) ListApp(ctx context.Context) ([]*types.AppGroup, error) {
	if f.saved == nil {
		return []*types.AppGroup{}, nil
	}
	cp := *f.saved
	return []*types.AppGroup{&cp}, nil
}

func (f *fakeAppStore) DeleteService(ctx context.Context, appName string, serviceName string) error {
	return nil
}

func newLeaderAPIForAppTests(s store.Store) *Api {
	m := &Manager{ID: "manager-1", Store: s, AdvertiseAddr: "127.0.0.1:5556"}
	m.setRole(ManagerRoleLeader)
	m.AppController = control.NewAppController(s)
	return &Api{Manager: m}
}

func validAppPayload() string {
	return `{
		"Name": "shop",
		"Service": {
			"frontend": {
				"Name": "frontend",
				"Image": "nginx:latest",
				"CPU": 1,
				"Memory": 128,
				"Disk": 2,
				"Replicas": 1
			}
		},
		"Dependencies": {}
	}`
}

func TestCreateAppHandlerSuccess(t *testing.T) {
	fs := &fakeAppStore{}
	api := newLeaderAPIForAppTests(fs)

	req := httptest.NewRequest(http.MethodPost, "/app", strings.NewReader(validAppPayload()))
	w := httptest.NewRecorder()

	api.CreateAppHandler(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d", http.StatusCreated, w.Code)
	}
	if fs.saved == nil {
		t.Fatal("expected app to be saved")
	}
	if fs.saved.Name != "shop" {
		t.Fatalf("expected saved app name shop, got %q", fs.saved.Name)
	}
}

func TestCreateAppHandlerRejectsUnknownField(t *testing.T) {
	fs := &fakeAppStore{}
	api := newLeaderAPIForAppTests(fs)

	payload := `{
		"Name": "shop",
		"Service": {},
		"Unknown": 1
	}`
	req := httptest.NewRequest(http.MethodPost, "/app", strings.NewReader(payload))
	w := httptest.NewRecorder()

	api.CreateAppHandler(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestCreateAppHandlerRejectsMissingName(t *testing.T) {
	fs := &fakeAppStore{}
	api := newLeaderAPIForAppTests(fs)

	payload := `{
		"Service": {
			"frontend": {
				"Name": "frontend",
				"Image": "nginx:latest",
				"CPU": 1,
				"Memory": 128,
				"Disk": 2,
				"Replicas": 1
			}
		}
	}`
	req := httptest.NewRequest(http.MethodPost, "/app", strings.NewReader(payload))
	w := httptest.NewRecorder()

	api.CreateAppHandler(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestCreateAppHandlerControllerUnavailable(t *testing.T) {
	m := &Manager{ID: "manager-1", Store: &fakeAppStore{}, AdvertiseAddr: "127.0.0.1:5556"}
	m.setRole(ManagerRoleLeader)
	api := &Api{Manager: m}

	req := httptest.NewRequest(http.MethodPost, "/app", strings.NewReader(validAppPayload()))
	w := httptest.NewRecorder()

	api.CreateAppHandler(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected status %d, got %d", http.StatusServiceUnavailable, w.Code)
	}
}

func TestCreateAppHandlerCreateFailure(t *testing.T) {
	fs := &fakeAppStore{saveErr: errors.New("store down")}
	api := newLeaderAPIForAppTests(fs)

	req := httptest.NewRequest(http.MethodPost, "/app", strings.NewReader(validAppPayload()))
	w := httptest.NewRecorder()

	api.CreateAppHandler(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestCreateAppHandlerFollowerForwardingFailure(t *testing.T) {
	m := &Manager{ID: "manager-2", Store: &fakeAppStore{}}
	m.setRole(ManagerRoleFollower)
	api := &Api{Manager: m}

	req := httptest.NewRequest(http.MethodPost, "/app", strings.NewReader(validAppPayload()))
	w := httptest.NewRecorder()

	api.CreateAppHandler(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected status %d, got %d", http.StatusServiceUnavailable, w.Code)
	}

	var resp ErrResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("expected JSON error response, got %v", err)
	}
	if resp.Message == "" {
		t.Fatal("expected non-empty error message")
	}
}

func TestCreateAppRouteViaRouterSuccess(t *testing.T) {
	fs := &fakeAppStore{}
	api := newLeaderAPIForAppTests(fs)
	api.initRouter()

	req := httptest.NewRequest(http.MethodPost, "/app/", strings.NewReader(validAppPayload()))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	api.Router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d", http.StatusCreated, w.Code)
	}
	if fs.saved == nil {
		t.Fatal("expected app to be saved via router path")
	}
}

func TestCreateAppRouteViaRouterBadPayload(t *testing.T) {
	fs := &fakeAppStore{}
	api := newLeaderAPIForAppTests(fs)
	api.initRouter()

	req := httptest.NewRequest(http.MethodPost, "/app/", strings.NewReader(`{"Name":1}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	api.Router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}
