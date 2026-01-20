package worker

import (
	"fmt"
	"log"
	"time"
	"errors"
	"net/http"
	"encoding/json"
	"orion/cmd/pkg/task"
	"github.com/go-chi/chi"
	"github.com/google/uuid"
	"github.com/golang-collections/collections/queue"
)

type Worker struct {
	Name string
	Queue queue.Queue
	Db map[uuid.UUID]*task.Task
	TaskCount int
	Stats *Stats
}

func (w *Worker) AddTask(t *task.Task){
	w.Queue.Enqueue(t)
}

func(w *Worker) CollectStats() {
	for {
		log.Println("Collecting stats")
		w.Stats=GetStats()
		w.Stats.TaskCount=w.TaskCount
		time.Sleep(15*time.Second)
	}
}

func(w *Worker) GetTasks()[] *task.Task{
	tasks:=make([]*task.Task,0,len(w.Db))
	for _,t:=range w.Db{
		tasks=append(tasks,t)
	}
	return tasks;
}

func(w *Worker) RunTask() task.DockerResult{
	fmt.Println("To stop or run a task")
	t:=w.Queue.Dequeue()
	if t==nil {
		log.Println("No tasks in the queue")
		return task.DockerResult{Error:nil}
	}
	taskQueued,_:=t.(*task.Task)
	taskPersisted:=w.Db[taskQueued.ID]
	if taskPersisted==nil{
		taskPersisted = taskQueued
		w.Db[taskPersisted.ID]=taskQueued
		w.TaskCount=len(w.Db)
	}
	var result task.DockerResult
	if task.ValidStateTransition(taskPersisted.State,taskQueued.State){
	   switch taskQueued.State{
	    case task.Scheduled:
		  result=w.StartTask(taskQueued)
		case task.Completed:
			result=w.StopTask(taskQueued)
		default:
			result.Error=errors.New("We should not get here") 
	   }
	}else{
		err:=fmt.Errorf("Invalid transition from %v to %v",taskPersisted.State,taskQueued.State)
		result.Error=err
	}
	return result
}

func(w *Worker) StartTask(t *task.Task) task.DockerResult{
	fmt.Println("To start a task")
	t.StartTime=time.Now().UTC()
	config:=task.NewConfig(t)
	d:=task.NewDocker(&config)
	result:=d.Run()
	if result.Error!=nil{
		fmt.Printf("Err running task %v:%v\n",t.ID,result.Error)
		t.State=task.Failed
		w.Db[t.ID]=t
		return result
	}
	t.ContainerId=result.ContainerId
	t.State=task.Running
	w.Db[t.ID]=t
	return result
}

func(w *Worker) StopTask(t *task.Task) task.DockerResult{
	fmt.Println("To stop a task")
	config:=task.NewConfig(t)
	d:=task.NewDocker(&config)
	result:=d.Stop(t.ContainerId)
	if result.Error!=nil{
		log.Printf("Error in stopping the container %v:%v\n",t.ContainerId,result.Error)
		return result;
	}
	t.FinishTime=time.Now().UTC()
	t.State=task.Completed
	//update the task in the Db if the worker 
	w.Db[t.ID]=t
	log.Printf("Container %v stopped and removed for the task %v\n",t.ContainerId,t.ID)
	return result;
}
