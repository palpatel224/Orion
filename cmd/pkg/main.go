package main

import(
	"orion/cmd/pkg/task"
	"fmt"
	"time"
	"os"
	"strconv"
	"net/http"
	"orion/cmd/pkg/worker"
	"github.com/google/uuid"
	"github.com/golang-collections/collections/queue"
)

func runTasks(w *worker.Worker){
	for{
		if w.Queue.Len()!=0{
			t:=w.Queue.Peek().(*task.Task)
			t.State=task.Scheduled
			result:=w.RunTask()
			if result.Error!=nil{
				log.Printf("Error running tasks : %v\n",result.Error)
				panic(result.Error)
			}
			fmt.Println(result)
			fmt.Printf("Task %s is running in container %s\n",t.ContainerId,result.ContainerId)
			fmt.Printf("Time to sleep\n")
			time.Sleep(time.Second * 30)
			res:=w.StopTask(t)
			if res.Error!=nil{
				panic(res.Error)
			}
			fmt.Printf("Stopped successfully %s\n",res.ContainerId)
		}else{
			log.Printf("No tasks to process currently \n")
		}
        log.Println("Sleeping for 10 seconds.")
        time.Sleep(10 * time.Second)		
	}
}

func main(){
	host:=os.Getenv("HOST")
	port,_:=strconv.Atoi(os.Getenv("PORT"))

	t:=task.Task{
		ID:uuid.New(),
		Name:"Task-1",
		State:task.Pending,
		Image:"nginx:latest",
		Disk:2,
	}
	db:=make(map[uuid.UUID]*task.Task)
	w:=worker.Worker{
		Queue:*queue.New(),
		Db:db,
	}
	fmt.Println("Starting worker.....")

    w.AddTask(&t)

	api:=worker.Api{
		Address:host,
		Worker:&w,
		Port:port,
	}
	go runTasks(&w)
	// go w.CollectStats()
	api.Start()
}