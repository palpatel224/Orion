package metrics

import "github.com/prometheus/client_golang/prometheus"

var NodeSelectionLatency = prometheus.NewHistogram(
	prometheus.HistogramOpts{
		Name: "scheduler_node_selection_latency_seconds",
		Help: "Time taken by scheduler to select a node",
		Buckets: prometheus.DefBuckets,
	},
)

var TasksScheduled = prometheus.NewCounter(
	prometheus.CounterOpts{
		Name: "scheduler_tasks_total",
		Help: "Total tasks scheduled",
	},
)

var SchedulerLatency = prometheus.NewHistogram(
	prometheus.HistogramOpts{
		Name: "scheduler_latency_seconds",
		Help: "Time taken to schedule tasks",
	},
)

var SchedulerFailedTotal = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "scheduler_failed_total",
		Help: "How many times the  scheduler is not able to place the task in the worker's queue",
	},
	[]string{"node"},
)

var NodeSelections = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "scheduler_node_selection_total",
		Help: "How many times each node was selected",
	},
	[]string{"node"},
)

func Init() {
	prometheus.MustRegister(NodeSelectionLatency)
	prometheus.MustRegister(TasksScheduled)
	prometheus.MustRegister(SchedulerLatency)
	prometheus.MustRegister(NodeSelections)
	prometheus.MustRegister(SchedulerFailedTotal)
}