package worker

import (
	"fmt"
	"log"
	"time"
	"context"
	"errors"
	"github.com/google/uuid"
	"orchestrator/task"
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
	fmt.Printf("Added in the queue\n")
	w.Queue.Enqueue(t)
}

func(w *Worker) InspectTask(t task.Task) task.DockerInspectResponse{
	ctx:=context.Background()
	config:=task.NewConfig(&t)
	d:=task.NewDocker(&config)
	resp,err:= d.Client.ContainerInspect(ctx,t.ContainerId)
	return task.DockerInspectResponse{
		Container:&resp,
		Error:err,
	}
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

func (w *Worker) RunTask() {
	for {
		t := w.Queue.Dequeue()

		if t == nil {
			time.Sleep(200 * time.Millisecond)
			continue
		}

		taskQueued := t.(*task.Task)

		taskPersisted := w.Db[taskQueued.ID]
		if taskPersisted == nil {
			taskPersisted = taskQueued
			w.Db[taskPersisted.ID] = taskQueued
			w.TaskCount = len(w.Db)
		}

		var result task.DockerResult

		if task.ValidStateTransition(taskPersisted.State, taskQueued.State) {

			switch taskQueued.State {

			case task.Scheduled:
				result = w.StartTask(taskQueued)

			case task.Completed:
				result = w.StopTask(taskQueued)

			default:
				result.Error = errors.New("invalid state")
			}

		} else {

			err := fmt.Errorf("invalid transition %v -> %v",
				taskPersisted.State, taskQueued.State)
			result.Error = err
		}

		if result.Error != nil {
			log.Printf("Task execution error: %v", result.Error)
		}
	}
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

func(w *Worker) UpdateTasks(){
	for id,t:=range w.Db{
		if t.State == task.Running {
			resp:=w.InspectTask(*t)
			if resp.Error!=nil{
				fmt.Printf("Error : %v\n",resp.Error)
			}
			if resp.Container==nil{
				log.Printf("No container for running task %s\n",id)
				w.Db[id].State=task.Failed
			}
			if resp.Container.State.Status=="exited"{
				log.Printf("Container for task %s in non-running state %s",id,resp.Container.State.Status)
				w.Db[id].State=task.Failed
			}
			w.Db[id].PortBindings=resp.Container.NetworkSettings.NetworkSettingsBase.Ports
		}
	}
}