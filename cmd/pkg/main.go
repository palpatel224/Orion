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
			result:=w.RunTask()
			if result.Error!=nil{
				log.Printf("Error running tasks : %v\n",result.Error)
			}
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

	fmt.Println("Starting worker.....")

	w:=worker.Worker{
		Queue:*queue.New(),
		Db:make(map[uuid.UUID]*task.Task),
	}

	api:=worker.Api{
		Address:host,
		Worker:w,
		Port:port,
	}
	go runTasks(&w)
	go w.CollectStats()
	api.Start()
}