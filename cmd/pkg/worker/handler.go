package worker

import(
	"encoding/json"
	"net/http"
	"github.com/go-chi/chi"
	"log"
	"time"
	"bytes"
	"io"
	"fmt"
	"orchestrator/task"
	"github.com/google/uuid"
)

func(a *Api) HealthHandler(w http.ResponseWriter,r *http.Request){
	w.WriteHeader(http.StatusOK)
    w.Write([]byte("ok"))
	return
}

func(a *Api) MeasureBandwidthHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Target string `json:"target"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	payload := bytes.Repeat([]byte("a"), 5*1024*1024) // 5MB
	start := time.Now()
	url := fmt.Sprintf("http://%s/bandwidth-receive", req.Target)
	http.Post(url, "application/octet-stream", bytes.NewReader(payload))
	duration := time.Since(start).Seconds()
	bandwidth := float64(len(payload)) / duration / (1024 * 1024)
	resp := map[string]float64{
		"bandwidth": bandwidth,
	}
	json.NewEncoder(w).Encode(resp)
}

func(a *Api) BandwidthRecieverHandler(w http.ResponseWriter,r *http.Request){
	io.Copy(io.Discard,r.Body)
	w.WriteHeader(http.StatusCreated)
}

func(a *Api) GetStatsHandler(w http.ResponseWriter,r *http.Request){
	w.Header().Set("Content-Type","application/json")
	w.WriteHeader(200)
	err:=json.NewEncoder(w).Encode(a.Worker.Stats)
	if err!=nil {
		http.Error(w,"Failed to encode",http.StatusInternalServerError)
		return
	}
}

func(a *Api) StartTaskHandler(w http.ResponseWriter,r *http.Request){
	d:=json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	w.Header().Set("Content-Type","application/json")
	te:=task.TaskEvent{}
	err:=d.Decode(&te)
	fmt.Printf("After decoding i am getting %+v\n",te)
	if err!=nil {
		msg:=fmt.Sprintf("Error unmarshalling body : %v\n",err)
		log.Printf(msg)
		w.WriteHeader(400)
		e:=ErrResponse{
			HTTPStatusCode:400,
			Message:msg,
		}
		json.NewEncoder(w).Encode(e)
		return
	}
	a.Worker.AddTask(&te.Task)
	log.Printf("Added task %v\n",te.Task.ID)
	w.WriteHeader(201)
	json.NewEncoder(w).Encode(te.Task)
}

func(a *Api) GetTasksHandler(w http.ResponseWriter,r *http.Request){
	w.Header().Set("Content-Type","application/json")
	w.WriteHeader(200)
	json.NewEncoder(w).Encode(a.Worker.GetTasks())
}

func(a *Api) StopTaskHandler(w http.ResponseWriter,r *http.Request){
	taskID := chi.URLParam(r,"taskID")
	if taskID==""{
		log.Printf("No taskID passed in request. \n")
		w.WriteHeader(400)
		return;
	}
	tID,_:=uuid.Parse(taskID)
	_,ok:=a.Worker.Db[tID]
	if !ok{
		log.Printf("No task with ID %v found",tID)
		w.WriteHeader(404)
		return;
	}
	taskToStop:=*a.Worker.Db[tID]
	//changing the state in the desired state of tasks (for queue) not in the current state of tasks (Db)
	taskToStop.State=task.Completed
	a.Worker.AddTask(&taskToStop)
	log.Printf("Added task %v to stop container %v\n",taskToStop.ID,taskToStop.ContainerId)
	w.WriteHeader(204)
}