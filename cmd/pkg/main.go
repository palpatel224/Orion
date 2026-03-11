package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/golang-collections/collections/queue"
	"github.com/google/uuid"

	"orchestrator/manager"
	"orchestrator/store"
	"orchestrator/task"
	"orchestrator/types"
	"orchestrator/worker"
)

func startWorkers(host string) ([]store.Worker, []*worker.Worker) {

	basePort := 5555
	workers := []store.Worker{}
	workerObjs := []*worker.Worker{}

	for i := 0; i < 3; i++ {

		w := &worker.Worker{
			Name:  uuid.New().String(),
			Queue: *queue.New(),
			Db:    make(map[uuid.UUID]*task.Task),
		}

		go w.CollectStats()

		api := worker.Api{
			Address: host,
			Port:    basePort + i,
			Worker:  w,
		}

		go api.Start()
		go w.RunTask()
		go w.UpdateTasks()

		addr := fmt.Sprintf("%s:%d", host, basePort+i)

		workers = append(workers, store.Worker{ID:w.Name,Address:addr,Heartbeat:time.Now()})
		workerObjs = append(workerObjs, w)
	}

	return workers, workerObjs
}

func startManagers(store store.Store) (*manager.Manager, []*manager.Manager) {

	leaderAddr := "127.0.0.1:8080"

	leader := manager.NewWithConfig(manager.Config{
		ID:            uuid.New().String(),
		AdvertiseAddr: leaderAddr,
		Store:         store,
	})

	mapi := manager.Api{
		Address: "127.0.0.1",
		Port:    8080,
		Manager: leader,
	}

	go mapi.Start()
	// go leader.UpdateTasks()
	go leader.DoHealthChecks()

	followers := []*manager.Manager{}

	ports := []int{8081, 8082}

	for _, port := range ports {

		addr := fmt.Sprintf("127.0.0.1:%d", port)

		m := manager.NewWithConfig(manager.Config{
			ID:            uuid.New().String(),
			AdvertiseAddr: addr,
			Store:         store,
		})

		api := manager.Api{
			Address: "127.0.0.1",
			Port:    port,
			Manager: m,
		}

		go api.Start()
		go m.UpdateTasks()
		go m.DoHealthChecks()

		followers = append(followers, m)
	}

	return leader, followers
}

func registerWorkers(ctx context.Context, leaderAddr string, workerObjs []*worker.Worker, workers []store.Worker) {

	for i, _ := range workerObjs {

		for {
			err := worker.RegisterWithManager(ctx, leaderAddr, workers[i])
			if err == nil {
				fmt.Println("Worker registered:", workers[i].ID)
				break
			}

			fmt.Println("Retrying worker registration...")
			time.Sleep(2 * time.Second)
		}

		go worker.StartHeartbeat(ctx, leaderAddr, workers[i].ID, 2*time.Second)
	}
}

func submitApp() {

	app := types.AppGroup{
		Name:    "App-1",
		Version: 1,
		Service: map[string]*types.ServiceSpec{

			"frontend": {
				Name:     "frontend",
				Image:    "nginx:latest",
				CPU:      1,
				Memory:   128*1024*1024,
				Disk:     2,
				Replicas: 1,
			},

			"cache": {
				Name:     "cache",
				Image:    "redis:latest",
				CPU:      1,
				Memory:   128*1024*1024,
				Disk:     2,
				Replicas: 1,
			},

			"worker": {
				Name:     "worker",
				Image:    "ubuntu:latest",
				CPU:      1,
				Memory:   128*1024*1024,
				Disk:     2,
				Replicas: 1,
			},
		},
		Dependencies: map[string][]types.Dependency{
            "frontend": {
               {
                  TargetService: "cache",
                  MaxNetworkCost: 1,
               },
            },
            "cache": {
                {
                  TargetService: "worker",
                  MaxNetworkCost: 1,
            },
        },
    },

	}

	data, _ := json.Marshal(app)

	resp, err := http.Post(
		"http://127.0.0.1:8080/app",
		"application/json",
		bytes.NewBuffer(data),
	)

	if err != nil {
		log.Println("Submit failed:", err)
		return
	}

	defer resp.Body.Close()

	fmt.Println("Submit status:", resp.Status)
}

func main() {

	ctx := context.Background()

	// 1 Start workers
	workers, workerObjs := startWorkers("localhost")

	// 2 Create etcd store
	etcdStore, err := store.NewEtcdStore([]string{"localhost:2379"})
	if err != nil {
		log.Fatal(err)
	}

	// 3 Start managers
	leader, followers := startManagers(etcdStore)

	fmt.Println("Managers started:", followers)

	// 4 Wait for leader election
	time.Sleep(6 * time.Second)

	// 5 Register workers
	registerWorkers(ctx, leader.AdvertiseAddr, workerObjs, workers)

	// 6 Submit application
	time.Sleep(2 * time.Second)
	submitApp()

	select {}
}