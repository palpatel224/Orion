package task

import(
	"github.com/google/uuid"
	"github.com/docker/go-connections/nat"
	"time"
	"io"
	"os"
	"context"
	"math"
	"github.com/docker/docker/client"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
)

type State int

const (
	Pending State = iota
	Scheduled
	Running
	Completed
	Failed
)

type Task struct{
	ID uuid.UUID
	ContainerId uuid.UUID
	Name string
	State State
	Image string
	Memory int64
	Disk int64
	ExposedPorts nat.PortSet
	PortBindings  map[string]string
	RestartPolicy string
	StartTime     time.Time
	FinishTime    time.Time
}

type TaskEvent struct {
   ID uuid.UUID
   State State
   Timestamp time.Time
   Task Task
}

type Config struct{
	Name string
	AttachStdin bool
	AttachStdout bool
	AttachStderr bool
	ExposedPorts nat.PortSet
	Cmd []string
	Image string
	Cpu float64
	Memory int64
	Disk int64
	Env []string
	RestartPolicy string
}

type DockerResult struct {
   Error error
   Action string
   ContainerId string
   Result string
}

type Docker struct {
	Client *client.Client
	Config Config
}

func(d *Docker) Run() DockerResult {
	ctx:=context.Background()
	reader,err:=d.Client.ImagePull(ctx context.Context,refStr string,options types.PullOptions )
	if err!=nil {
		log.Printf("Error pulling image %s : %v\n",d.Config.Image,err)
		return DockerResult{Error:err}
	}
	io.Copy(os.Stdout,reader)

	rp:=container.RestartPolicy{
		Name:d.Config.RestartPolicy,
	}

	r:=container.Resources{
		Memory:d.Confg.Memory,
		NanoCPUs:int64(d.Config,CPu*math.Pow(10,9)),
	}

	cc:=container.Config{
		Image:d.Config.Image,
		Tty:false,
		Env: d.Config.Env,
		ExposedPorts:d.Config.ExposedPorts,
	}

	hc:=container.HostConfig{
		RestartPolicy:rp,
		Resources:r,
		PublishAllPorts:true,
	}

	res,er:=d.Client.ContainerCreate(ctx,&cc,&hc,nil,nil,d.Config.Name)
	if er!=nil{
		log.Printf("Error creating container using image %s :%v\n",d.Config.Image,er)
		return DockerResult{Error:err}
	}

	err = d.Client.ContainerStart(ctx,res.ID,types.ContainerStartOptions{})
	if err!=nil {
		log.Printf("Error starting container %s : %v\n",resp.ID,err)
		return DockerResult{Error:err}
	}

	out,e:=d.Client.ContainerLogs(ctx,res.ID,container.LogsOptions{ShowStdout:true,ShowStderr:true})
	if e!=nil{
		log.Printf("Error getting logs for the container %s : %v\n",res.ID,e)
		return DockerResult{Error:e}
	}
	stdcopy.StdCopy(os.Stdout,os.Stderr,out)
	return DockerResult{containerId:res.ID,Action:"start",Result:"success"}
}

func(d *Docker) Stop(id String) DockerResult{
	ctx:=context.Background()
	err:=d.Clinet.ContainerStop(ctx,id,nil)
	if err!=nil{
		log.Printf("Error stopping container %d : %v\n",id,err)
		return DockerResult{Error:err}
	}
	err=d.Client.ContainerRemove(ctx,id,container.RemoveOptions{
		RemoveVolumes:true,
		RemoveLinks:false,
		Force:false,
	})
	if err!=nil{
		log.Printf("Error in removing the container %d : %s\n",id,err)
		return DockerResult{Error:err}
	}
	return DockerResult{Action:"stop",Result:"success",Error:nil}
}

