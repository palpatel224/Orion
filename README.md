# ORION 🌌

**Network-Aware Container Orchestrator**

[![Go Version](https://img.shields.io/badge/Go-1.25.3-blue.svg)](https://golang.org/)
[![gRPC](https://img.shields.io/badge/gRPC-v1.77.0-green.svg)](https://grpc.io/)
[![License](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

> A lightweight, distributed container orchestrator built from the ground up with network topology awareness and fault tolerance at its core.

**Authors:** Pal Patel (231CS240) • Aditi Pandey (231IT003)

---

## Overview

ORION is a production-grade container orchestration platform designed to address the critical gap in modern orchestrators: **network-aware scheduling**. Unlike traditional orchestrators that treat the network as an afterthought, ORION makes network topology, latency, and bandwidth first-class citizens in scheduling decisions.

Similar to K3s in its lightweight approach, ORION goes further by implementing Raft-based consensus for fault-tolerant state management and incorporating real-time network metrics into every scheduling decision. The result is a self-healing orchestrator that optimally places containers based on actual network conditions, not just CPU and memory.

### Why ORION?

**The Problem:** Modern container orchestrators like Kubernetes can struggle with:
- Network-blind scheduling leading to suboptimal container placement
- Cascading failures when network partitions occur
- Inconsistent state during complex failure scenarios
- High overhead for edge and IoT deployments

**The Solution:** ORION provides:
- **Network-Aware Scheduling:** Real-time network topology analysis and latency-based scoring
- **Consensus-Driven Architecture:** Raft consensus ensures all scheduling decisions are durable and replicated
- **Lightweight Design:** Single-binary deployment with minimal resource footprint
- **Self-Healing:** Automatic detection and recovery from node and network failures

---

## Architecture

ORION consists of four core components working in harmony:

### 1. **Consensus Core** (Raft-based State Machine)
The distributed brain of ORION. A cluster of nodes maintains a replicated, consistent log of all scheduling decisions using the Raft consensus algorithm.

- **Leader Election:** Automatic failover when leader nodes fail
- **Log Replication:** All scheduling decisions replicated across the cluster
- **Fault Tolerance:** Survives `(n-1)/2` node failures in an `n`-node cluster

### 2. **Network Monitor & Scorer**
The network-awareness engine that continuously monitors network topology and conditions.

- **Real-time Network Scanning:** Active probing of latency, bandwidth, and packet loss
- **Topology Discovery:** Automatic detection of network segments and zones
- **Intelligent Scoring:** Multi-factor scoring algorithm considering:
  - Inter-node latency
  - Available bandwidth
  - Network stability metrics
  - Geographical proximity

### 3. **Custom Container Runtime** (OCI-Compliant)
Lightweight container runtime built on Linux kernel primitives.

- **Linux Namespaces:** Process, network, mount, and user isolation
- **Cgroups:** Resource limiting and accounting
- **OCI Specification:** Full compatibility with standard container images
- **Minimal Overhead:** Direct kernel integration without Docker dependency

### 4. **Distributed Worker Fleet**
Worker nodes that execute containerized workloads.

- **Agent-based Architecture:** Each worker runs a lightweight agent
- **Heartbeat Protocol:** Continuous health reporting to consensus core
- **Dynamic Registration:** Workers auto-register and deregister gracefully
- **Task Execution:** Receive and execute scheduling decisions from the leader

### 5. **Control Plane API** (gRPC)
High-performance API for cluster management and workload submission.

- **gRPC Communication:** Efficient binary protocol with Protocol Buffers
- **RESTful Gateway:** Optional HTTP/JSON gateway for compatibility
- **Authentication & Authorization:** Secure cluster access control
- **Observability:** Built-in metrics and tracing

---

## Key Features

### Network-Aware Scheduling
ORION's scheduler evaluates network conditions in real-time:
```
Score(node) = α·CPU_Score + β·Memory_Score + γ·Network_Score
```
Where `Network_Score` considers:
- Latency to dependent services
- Available bandwidth
- Historical network stability
- Network zone affinity

### Consensus-Driven Operations
Every scheduling decision is committed to the Raft log before execution:
```
1. API Request → Control Plane
2. Proposal → Raft Leader
3. Replication → Raft Followers
4. Commit → Durable State
5. Execute → Worker Nodes
```

### Self-Healing & Reconciliation
The **Orchestration Nexus** continuously reconciles desired vs. actual state:
- **Desired State:** What should be running (from Raft log)
- **Actual State:** What is running (from worker heartbeats)
- **Reconciliation:** Automatic correction of drift

### Failure Resilience
- **Node Failures:** Automatic workload rescheduling
- **Network Partitions:** Raft ensures split-brain prevention
- **Leader Failures:** Sub-second leader election and failover
- **Partial Failures:** Graceful degradation and recovery

---

## Installation

### Prerequisites
- Go 1.25.3 or higher
- Linux kernel 4.4+ (for namespace support)
- Protocol Buffers compiler (`protoc`)

### Build from Source
```bash
# Clone the repository
git clone https://github.com/palpatel224/Orion.git
cd Orion

# Install dependencies
go mod download

# Generate gRPC code
make proto

# Build the binary
make build

# Install system-wide (optional)
sudo make install
```

### Quick Start
```bash
# Start a single-node cluster (for testing)
orion start --mode=single

# Start a multi-node cluster
# On node 1 (initial leader)
orion start --cluster-id=orion-cluster --node-id=node1 --bind=10.0.0.1:7000

# On node 2
orion start --cluster-id=orion-cluster --node-id=node2 --bind=10.0.0.2:7000 --join=10.0.0.1:7000

# On node 3
orion start --cluster-id=orion-cluster --node-id=node3 --bind=10.0.0.3:7000 --join=10.0.0.1:7000
```

---

## Usage

### Deploy a Container
```bash
# Deploy a simple web service
orion deploy \
  --image=nginx:latest \
  --name=web \
  --replicas=3 \
  --port=80 \
  --network-affinity=zone-a

# Check deployment status
orion status web

# View container logs
orion logs web
```

### Network-Aware Scheduling Example
```yaml
# deployment.yaml
apiVersion: orion.io/v1
kind: Deployment
metadata:
  name: api-gateway
spec:
  replicas: 3
  container:
    image: api-gateway:v1.0
    ports:
      - 8080
  scheduling:
    networkAware: true
    affinityRules:
      - type: latency
        target: database-cluster
        maxLatency: 5ms
      - type: bandwidth
        minBandwidth: 100Mbps
    antiAffinityRules:
      - type: network-zone
        avoid: edge-zones
```

```bash
orion apply -f deployment.yaml
```

---

## Configuration

### Cluster Configuration
```yaml
# orion.yaml
cluster:
  id: production-cluster
  consensus:
    electionTimeout: 1000ms
    heartbeatInterval: 100ms
    snapshotInterval: 1h
  
network:
  monitoring:
    enabled: true
    scanInterval: 30s
    latencyProbes: 10
  scoring:
    weights:
      cpu: 0.3
      memory: 0.3
      network: 0.4

runtime:
  containerBackend: native  # or "containerd"
  ociCompliant: true
  resourceLimits:
    maxContainersPerNode: 100
```

---

## Testing & Chaos Engineering

ORION includes built-in chaos testing capabilities:

```bash
# Simulate node failure
orion chaos node-failure --target=node2 --duration=60s

# Simulate network partition
orion chaos network-partition --partition=node1,node2:node3 --duration=30s

# Simulate latency injection
orion chaos latency --increase=100ms --nodes=all --duration=2m

# Run chaos monkey (random failures)
orion chaos monkey --interval=5m --severity=medium
```

---

## Monitoring & Observability

### Metrics
ORION exposes Prometheus-compatible metrics:
- Cluster health and consensus state
- Scheduling latency and success rate
- Network topology metrics
- Container resource utilization

### Logging
Structured logging with configurable levels:
```bash
orion --log-level=debug --log-format=json
```

### Tracing
Distributed tracing with OpenTelemetry support for request flow visualization.

---

## Roadmap

- [ ] **v0.1** - Core Raft implementation and basic container runtime
- [ ] **v0.2** - Network monitoring and topology discovery
- [ ] **v0.3** - Network-aware scheduler with multi-factor scoring
- [ ] **v0.4** - Production-ready with chaos testing suite
- [ ] **v1.0** - Full feature parity with K3s + network awareness
- [ ] **v1.1** - Service mesh integration
- [ ] **v1.2** - GPU workload support
- [ ] **v2.0** - Multi-cluster federation

---

## Contributing

We welcome contributions! Please see [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

### Development Setup
```bash
# Install development dependencies
make dev-deps

# Run tests
make test

# Run linters
make lint

# Start development cluster
make dev-cluster
```

---

## Documentation

- [Architecture Deep Dive](docs/architecture.md)
- [Raft Consensus Implementation](docs/consensus.md)
- [Network-Aware Scheduling Algorithm](docs/scheduling.md)
- [OCI Runtime Specification](docs/runtime.md)
- [API Reference](docs/api.md)
- [Operations Guide](docs/operations.md)

---

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

---

## Acknowledgments

- The Raft consensus algorithm by Diego Ongaro and John Ousterhout
- The Open Container Initiative (OCI) for container standards
- The Kubernetes community for orchestration patterns
- K3s for lightweight orchestrator inspiration

---

## Contact

**Pal Patel** - 231CS240  
**Aditi Pandey** - 231IT003

Project Link: [https://github.com/palpatel224/Orion](https://github.com/palpatel224/Orion)

