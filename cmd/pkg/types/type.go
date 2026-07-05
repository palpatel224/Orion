package types

import (
	"github.com/docker/go-connections/nat"
)

type AppStatus string

type HealthCheckType string

const (
    HealthCheckHTTP HealthCheckType = "http"
    HealthCheckTCP  HealthCheckType = "tcp"
    HealthCheckNone HealthCheckType = ""
)

const(
	AppDraft AppStatus = "Draft"
	AppActive AppStatus ="Active"
)

type Dependency struct {
	TargetService string
	MaxNetworkCost int
}

type ServiceSpec struct {
	Name     string
	Image    string
	CPU      int64
	Memory   int64
	Disk     int64
	Replicas int
	ExposedPorts nat.PortSet  
	Cmd      []string
	HealthCheck string
	HealthCheckType HealthCheckType
}

type AppGroup struct {
	Name string
	Version int
	Service map[string]*ServiceSpec
	Dependencies map[string][]Dependency
	Status AppStatus
}
