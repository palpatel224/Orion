package manager

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"errors"
	"context"
	"time"
	"orchestrator/task"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"orchestrator/store"
)

func (a *Api) ForwardToLeader(w http.ResponseWriter,r *http.Request) bool{
	if a.Manager==nil || a.Manager.isLeader(){
		return false
	}
	ctx,cancel:=context.WithTimeout(r.Context(),5*time.Second)
	defer cancel()

	addr,err:=a.Manager.LeaderAddress(ctx)
	if err!=nil || addr==""{
		msg := "leader unavailable; cannot forward request"
		if err != nil {
			msg = fmt.Sprintf("leader unavailable: %v", err)
		}
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(ErrResponse{HTTPStatusCode: http.StatusServiceUnavailable, Message: msg})
		return true
	}

	targetURL:=fmt.Sprintf("http://%s%s",addr,r.URL.RequestURI())
	req,err:=http.NewRequestWithContext(ctx,r.Method,targetURL,r.Body)
	if err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(ErrResponse{HTTPStatusCode: http.StatusServiceUnavailable, Message: "failed to build forward request"})
		return true
	}
	req.Header=r.Header.Clone()

	resp,err:=a.HTTPClient().Do(req)
	//headers recieved but response is still coming from network
	if err!=nil{
		http.Redirect(w,req,targetURL, http.StatusTemporaryRedirect)
		return true
	}
	//once reading is done close the connections
	defer resp.Body.Close()

	for k, values := range resp.Header {
		for _, v := range values {
			w.Header().Add(k, v)
		}
	}

	w.WriteHeader(resp.StatusCode)
	if _, copyErr := io.Copy(w, resp.Body); copyErr != nil {
		log.Printf("Manager %s: failed to copy proxied response: %v", a.Manager.ID, copyErr)
	}

	return true
}

func(a *Api) RegisterWorkerHandler(w http.ResponseWriter,r *http.Request) {
	//If manager is not leader forward it and request will be handled
	if a.ForwardToLeader(w, r) {
		return
	}
	//If manager is a Leader
	if !a.Manager.isLeader() {
		msg := "manager is follower; worker registration disabled"
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(ErrResponse{HTTPStatusCode: http.StatusServiceUnavailable, Message: msg})
		return
	}
	if a.Manager.Store == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	var worker store.Worker
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&worker); err != nil {
		msg := fmt.Sprintf("Error unmarshalling body: %v", err)
		log.Print(msg)
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrResponse{HTTPStatusCode: http.StatusBadRequest, Message: msg})
		return
	}
	if worker.ID == "" || worker.Address == "" {
		msg := "worker id and address are required"
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrResponse{HTTPStatusCode: http.StatusBadRequest, Message: msg})
		return
	}
	worker.Heartbeat=time.Now().UTC()
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	if err := a.Manager.Store.RegisterWorker(ctx, worker); err != nil {
		msg := fmt.Sprintf("Error registering worker %s: %v", worker.ID, err)
		log.Print(msg)
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ErrResponse{HTTPStatusCode: http.StatusInternalServerError, Message: msg})
		return
	}

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(worker)
}

//Handler for updating Worker Heartbeat
func(a *Api) HeartbeatHandler(w http.ResponseWriter,r *http.Request){
	if a.ForwardToLeader(w,r){
		return
	}
	if !a.Manager.isLeader(){
		msg:="manager is a follower;heartbeat update is disabled"
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(ErrResponse{HTTPStatusCode:http.StatusServiceUnavailable,Message:msg})
		return
	}
	if a.Manager.Store == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	workerID:=chi.URLParam(r,"workerID")
	if workerID==""{
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrResponse{HTTPStatusCode: http.StatusBadRequest, Message: "worker id is required"})
		return
	}
	ctx,cancel:=context.WithTimeout(r.Context(),5*time.Second)
	defer cancel()
	if err:=a.Manager.Store.UpdateWorkerHeartbeat(ctx,workerID,time.Now().UTC()); err!=nil{
		if errors.Is(err,store.ErrNotFound){
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(ErrResponse{HTTPStatusCode:http.StatusNotFound,Message:"Worker is not registered"})
			return
		}
		msg := fmt.Sprintf("Error updating heartbeat for worker %s: %v", workerID, err)
		log.Print(msg)
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ErrResponse{HTTPStatusCode: http.StatusInternalServerError, Message: msg})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *Api) StartTaskHandler(w http.ResponseWriter, r *http.Request) {
	if a.ForwardToLeader(w,r){
		return
	}
	if !a.Manager.isLeader(){
		msg:="Manager is follower;Task creation disabled"
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(ErrResponse{HTTPStatusCode:http.StatusServiceUnavailable,Message:msg})
		return
	}
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()

	te := task.TaskEvent{}
	err := d.Decode(&te)

	if err != nil {
		msg := fmt.Sprintf("Error unmarshalling body: %v\n", err)
		log.Printf(msg)
		w.WriteHeader(400)
		e := ErrResponse{
			HTTPStatusCode: 400,
			Message:        msg,
		}
		json.NewEncoder(w).Encode(e)
		return
	}

	if er:=a.Manager.AddTask(te);er!=nil{
		msg:=fmt.Sprintf("Unable to add task : %v",er)
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(ErrResponse{HTTPStatusCode:http.StatusServiceUnavailable,Message:msg})
		return
	}

	log.Printf("Added task %v\n", te.Task.ID)
	w.WriteHeader(201)
	json.NewEncoder(w).Encode(te)
}

//check this start
func (a *Api) GetTasksHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(200)
	json.NewEncoder(w).Encode(a.Manager.GetTasks())
}
//check this end

func (a *Api) StopTaskHandler(w http.ResponseWriter, r *http.Request) {

	if a.ForwardToLeader(w,r){
		return
	}

	if !a.Manager.isLeader(){
		msg:=fmt.Sprintf("Manager is a follower;task stop diabled")
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(ErrResponse{HTTPStatusCode: http.StatusServiceUnavailable, Message: msg})
		return
	}

	taskID := chi.URLParam(r, "taskID")
	if taskID == "" {
		log.Printf("No taskID passed in request.\n")
		w.WriteHeader(400)
	}

	tID, _ := uuid.Parse(taskID)
	ctx,cancel:=context.WithTimeout(r.Context(),5*time.Second)
	defer cancel()
	existingTask,existingWorker,err:=a.Manager.etcdStore.GetTask(ctx,tID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			log.Printf("No task with ID %v found", tID)
			w.WriteHeader(404)
			return
		}
		log.Printf("Error retrieving task %v: %v", tID, err)
		w.WriteHeader(500)
		return
	}

	te := task.TaskEvent{
		ID:        uuid.New(),
		State:     task.Completed,
		Timestamp: time.Now(),
	}

	taskCopy := *existingTask
	taskCopy.State = task.Completed
	te.Task = taskCopy
	te.Task.RestartCount = existingTask.RestartCount
	
	// Preserve the worker assignment so it can be used during stop processing.
	if existingWorker != "" {
		if err := a.Manager.etcdStore.CreateTask(ctx, &te.Task, existingWorker); err != nil {
			log.Printf("Error persisting stop request for task %v: %v", tID, err)
		}
	}
	if err := a.Manager.AddTask(te); err != nil {
		msg := fmt.Sprintf("Unable to enqueue stop request: %v", err)
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(ErrResponse{HTTPStatusCode: http.StatusServiceUnavailable, Message: msg})
		return
	}

	log.Printf("Added task event %v to stop task %v\n", te.ID, existingTask.ID)
	w.WriteHeader(204)
}

// Returns the current role and Leader Address
func (a *Api) StatusHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	type statusResponse struct {
		ManagerID     string `json:"manager_id"`
		Role          string `json:"role"`
		LeaderAddress string `json:"leader_address,omitempty"`
	}

	resp := statusResponse{
		ManagerID: a.Manager.ID,
		Role:      string(a.Manager.CurrentRole()),
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	if addr, err := a.Manager.LeaderAddress(ctx); err == nil {
		resp.LeaderAddress = addr
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(resp)
}

func(a *Api) GetNodesHandler(w http.ResponseWriter,r *http.Request){
	w.Header().Set("Content-Type","application/json")
	if a.Manager.Store == nil{
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(ErrResponse{HTTPStatusCode: http.StatusServiceUnavailable, Message: "store not configured"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	workers, err := a.Manager.Store.ListWorkers(ctx)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ErrResponse{HTTPStatusCode: http.StatusInternalServerError, Message: fmt.Sprintf("error listing workers: %v", err)})
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(workers)
}