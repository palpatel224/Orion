package task

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"time"

	"github.com/docker/go-connections/nat"
	"github.com/google/uuid"

	"math"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
)

type State int

const (
	Pending State = iota
	Scheduled
	Running
	Completed
	Failed
)

type Task struct {
	ID            uuid.UUID
	ContainerId   string
	Name          string
	State         State
	Image         string
	CPU           int64
	Memory        int64
	Disk          int64
	ExposedPorts  nat.PortSet
	PortBindings  map[nat.Port][]nat.PortBinding
	RestartPolicy string
	StartTime     time.Time
	FinishTime    time.Time
	HealthCheck   string
	RestartCount  int
	AppName       string
	ServiceName   string
	Cmd           []string
}

func (s State) MarshalJSON() ([]byte, error) {
	names := []string{"Pending", "Scheduled", "Running", "Completed", "Failed"}
	return json.Marshal(names[s])
}

func (s *State) UnmarshalJSON(data []byte) error {
	var asInt int
	if err := json.Unmarshal(data, &asInt); err == nil {
		*s = State(asInt)
		return nil
	}

	var asString string
	if err := json.Unmarshal(data, &asString); err != nil {
		return err
	}

	switch asString {
	case "Pending":
		*s = Pending
	case "Scheduled":
		*s = Scheduled
	case "Running":
		*s = Running
	case "Completed":
		*s = Completed
	case "Failed":
		*s = Failed
	default:
		return fmt.Errorf("unknown task state: %s", asString)
	}

	return nil
}

type TaskEvent struct {
	ID        uuid.UUID
	State     State
	Timestamp time.Time
	Task      Task
}

type Config struct {
	Name          string
	AttachStdin   bool
	AttachStdout  bool
	AttachStderr  bool
	ExposedPorts  nat.PortSet
	Cmd           []string
	Image         string
	Cpu           float64
	Memory        int64
	Disk          int64
	Env           []string
	RestartPolicy string
}

type DockerInspectResponse struct {
	Container *types.ContainerJSON
	Error     error
}

type DockerResult struct {
	Error       error
	Action      string
	ContainerId string
	Result      string
}

type Docker struct {
	Client *client.Client
	Config Config
}

func NewConfig(t *Task) Config {
	c := Config{
		Name:          t.Name,
		AttachStdin:   true,
		AttachStderr:  true,
		AttachStdout:  true,
		ExposedPorts:  t.ExposedPorts,
		Image:         t.Image,
		Disk:          t.Disk,
		Memory:        t.Memory,
		RestartPolicy: t.RestartPolicy,
	}
	return c
}

func NewDocker(c *Config) Docker {
	cli, _ := client.NewClientWithOpts(client.FromEnv)
	ctx := context.Background()
	cli.NegotiateAPIVersion(ctx)
	d := Docker{
		Config: *c,
		Client: cli,
	}
	return d
}

func (d *Docker) Inspect(containerID string) DockerInspectResponse {
	dc, _ := client.NewClientWithOpts(client.FromEnv)
	ctx := context.Background()
	resp, err := dc.ContainerInspect(ctx, containerID)
	if err != nil {
		log.Printf("Error inspecting container : %s\n", err)
		return DockerInspectResponse{Error: err}
	}
	return DockerInspectResponse{Container: &resp}
}

func (d *Docker) Run() DockerResult {
	ctx := context.Background()
	reader, err := d.Client.ImagePull(ctx, d.Config.Image, image.PullOptions{})
	if err != nil {
		log.Printf("Error pulling image %s : %v\n", d.Config.Image, err)
		return DockerResult{Error: err}
	}
	io.Copy(os.Stdout, reader)

	rp := container.RestartPolicy{
		Name: container.RestartPolicyMode(d.Config.RestartPolicy),
	}
	memoryBytes:=d.Config.Memory*1024*1024
	r := container.Resources{
		Memory:   memoryBytes,
		NanoCPUs: int64(d.Config.Cpu * math.Pow(10, 9)),
	}

	cc := container.Config{
		Image:        d.Config.Image,
		Tty:          false,
		Env:          d.Config.Env,
		ExposedPorts: d.Config.ExposedPorts,
	}

	hc := container.HostConfig{
		RestartPolicy:   rp,
		Resources:       r,
		PublishAllPorts: true,
	}

	res, er := d.Client.ContainerCreate(ctx, &cc, &hc, nil, nil, d.Config.Name)
	if er != nil {
		log.Printf("Error creating container using image %s :%v\n", d.Config.Image, er)
		return DockerResult{Error: err}
	}

	err = d.Client.ContainerStart(ctx, res.ID, container.StartOptions{})
	if err != nil {
		log.Printf("Error starting container %s : %v\n", res.ID, err)
		return DockerResult{Error: err}
	}

	out, e := d.Client.ContainerLogs(ctx, res.ID, container.LogsOptions{ShowStdout: true, ShowStderr: true})
	if e != nil {
		log.Printf("Error getting logs for the container %s : %v\n", res.ID, e)
		return DockerResult{Error: e}
	}
	stdcopy.StdCopy(os.Stdout, os.Stderr, out)
	return DockerResult{ContainerId: res.ID, Action: "start", Result: "success"}
}

func (d *Docker) Stop(id string) DockerResult {
	ctx := context.Background()
	err := d.Client.ContainerStop(ctx, id, container.StopOptions{})
	if err != nil {
		log.Printf("Error stopping container %s : %v\n", id, err)
		return DockerResult{Error: err}
	}
	err = d.Client.ContainerRemove(ctx, id, container.RemoveOptions{
		RemoveVolumes: true,
		RemoveLinks:   false,
		Force:         false,
	})
	if err != nil {
		log.Printf("Error in removing the container %s : %s\n", id, err)
		return DockerResult{Error: err}
	}
	return DockerResult{Action: "stop", Result: "success", Error: nil}
}
