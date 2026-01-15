package worker

import (
	"fmt"
	"github.com/google/uuid"
	"github.com/golang-collections/collections/queue"
)

type Worker struct {
	Name string
	Queue queue.Queue
	Db map[uuid.UUID]*task.Task
	TaskCount int
}

func(w *Worker) CollectStats(){
	fmt.Println("For stats")
}

func(w *Worker) RunTask(){
	fmt.Println("To stop or run a task")
}

func(w *Worker) StartTask(){
	fmt.Println("To start a task")
}

func(w *Worker) StopTask(){
	fmt.Println("To stop a task")
}