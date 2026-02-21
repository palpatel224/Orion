package main

import(
	"log"
	"time"
	"context"
	"fmt"
	"orchestrator/store"
	"orchestrator/task"
	"orchestrator/manager"
	"orchestrator/worker"
	"github.com/google/uuid"
	"github.com/golang-collections/collections/queue"
)

func main(){

    w1 := worker.Worker{
		Name: uuid.New().String(),
		Queue: *queue.New(),
		Db:    make(map[uuid.UUID]*task.Task),
	}
	wport:=5555
	whost:=fmt.Sprintf("localhost")
	wapi1 := worker.Api{Address: whost, Port: wport, Worker: &w1}

	w2 := worker.Worker{
		Name: uuid.New().String(),
		Queue: *queue.New(),
		Db:    make(map[uuid.UUID]*task.Task),
	}
	wapi2 := worker.Api{Address: whost, Port: wport+1, Worker: &w2}

	w3 := worker.Worker{
		Name: uuid.New().String(),
		Queue: *queue.New(),
		Db:    make(map[uuid.UUID]*task.Task),
	}
	wapi3 := worker.Api{Address: whost, Port: wport+2, Worker: &w3}

	go wapi1.Start()
	go w1.RunTask()
	go w1.UpdateTasks()

	go wapi2.Start()
	go w2.RunTask()
	go w2.UpdateTasks()

	go wapi3.Start()
	go w3.RunTask()
	go w3.UpdateTasks()

	endpoints:=[]string{"localhost:2379"}
	etcdStore,err:=store.NewEtcdStore(endpoints)
	if err != nil {
		log.Fatalf("Failed to initialize etcd store: %v", err)
	}

    workers := []string{
		fmt.Sprintf("%s:%d", whost, wport),
		fmt.Sprintf("%s:%d", whost, wport+1),
		fmt.Sprintf("%s:%d", whost, wport+2),
	}

    //Start a leader
	msg:=fmt.Sprintf("127.0.0.1:%d",8080)
	leader:=manager.NewWithConfig(manager.Config{
		ID:uuid.New().String(),
		Workers:workers,
		AdvertiseAddr:msg,
		Store:etcdStore,
	})

	go leader.UpdateTasks()
	go leader.DoHealthChecks()

	//Creating 2 follower managers
	managers:=[]*manager.Manager{}
	ports := []int{8081, 8082}
	for _,port:=range ports{
		msg:=fmt.Sprintf("127.0.0.1:%d",port)
		cfg:=manager.Config{
			Workers:workers,
			ID:uuid.New().String(),
			AdvertiseAddr:msg,
			Store:etcdStore,
		}
		m:=manager.NewWithConfig(cfg)
		managers=append(managers,m)
	}

	mapi1:=manager.Api{Address:"127.0.0.1",Port:8080,Manager:leader}
	go mapi1.Start()
	mapi2:=manager.Api{Address:"127.0.0.1",Port:8081,Manager:managers[0]}
	go mapi2.Start()
	mapi3:=manager.Api{Address:"127.0.0.1",Port:8082,Manager:managers[1]}
	go mapi3.Start()
	fmt.Printf("Started listening")

    //Register worker with the manager
	ctx:=context.Background()

	time.Sleep(5*time.Second)
	meta1:=store.Worker{ID:w1.Name,Address:workers[0]}
	if err := worker.RegisterWithManager(ctx,leader.AdvertiseAddr,meta1); err != nil {
       log.Fatalf("registration failed: %v", err)
    }
	go worker.StartHeartbeat(ctx,leader.AdvertiseAddr,meta1.ID,2*time.Second)
	meta2:=store.Worker{ID:w2.Name,Address:workers[1]}
	if err :=worker.RegisterWithManager(ctx,leader.AdvertiseAddr,meta2); err != nil {
       log.Fatalf("registration failed: %v", err)
    }
	go worker.StartHeartbeat(ctx,leader.AdvertiseAddr,meta2.ID,2*time.Second)

	for _,m:=range managers{
		go m.UpdateTasks()
		go m.DoHealthChecks()
	}

	fmt.Printf("Managers are %v\n",managers)

	select{}
}