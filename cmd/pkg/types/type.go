package types

type AppStatus string

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
}

type AppGroup struct {
	Name string
	Version int
	Service map[string]*ServiceSpec
	Dependencies map[string][]Dependency
	Status AppStatus
}
