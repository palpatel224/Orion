# ORION 🌌

**Network-Aware Container Orchestrator**

[![Go Version](https://img.shields.io/badge/Go-1.24-blue.svg)](https://golang.org/)
[![License](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

> A lightweight, distributed container orchestrator built from the ground up with network topology awareness and fault tolerance at its core.

**Authors:** Pal Patel (231CS240) • Aditi Pandey (231IT003)

---

## Overview

ORION is a distributed container orchestration platform designed to address a critical gap in modern orchestrators: **network-aware scheduling**. Unlike traditional orchestrators that treat the network as an afterthought, ORION makes network topology, latency, and bandwidth first-class citizens in scheduling decisions.

ORION implements Raft-based consensus for fault-tolerant state management and incorporates real-time network metrics into every scheduling decision. The result is a self-healing orchestrator that optimally places containers based on actual network conditions, not just CPU and memory.

### Why ORION?

**The Problem:** Modern container orchestrators like Kubernetes struggle with:
- Network-blind scheduling leading to suboptimal container placement
- Inconsistent state during complex failure scenarios
- Cascading failures when a foundational service goes down

**The Solution:** ORION provides:
- **Network-Aware Scheduling:** Real-time latency and bandwidth-based scoring
- **Consensus-Driven Architecture:** Raft consensus ensures all state is durable and consistent
- **Dependency-Aware Deployment:** Microservices are deployed in correct dependency order
- **Self-Healing:** Automatic detection and recovery from node and task failures

---

## Architecture

ORION consists of four core components working in harmony:

### 1. Consensus Core (Raft-based State Machine)
The distributed brain of ORION. A cluster of manager nodes maintains consistent state using etcd's built-in Raft consensus algorithm.

- **Leader Election:** Automatic failover when leader nodes fail
- **Consistent State:** All cluster state persisted in etcd and replicated across managers
- **Fault Tolerance:** Survives `(n-1)/2` node failures in an `n`-node cluster
- **Request Forwarding:** Non-leader managers forward client requests to the elected leader

### 2. Network Monitor & Scorer
The network-awareness engine that continuously monitors network conditions.

- **Real-time Network Probing:** Active measurement of latency and bandwidth between nodes
- **Intelligent Scoring:** Multi-factor scoring algorithm considering:
  - Inter-node latency
  - Available bandwidth

### 3. App Controller & Scheduler
Handles dependency resolution and workload placement.

- **Dependency-Aware Deployment:** Uses Kahn's topological sort to resolve microservice deployment order; fails fast on circular dependencies
- **Composite Scoring:** Scheduler combines live CPU/memory base scores with network scores for optimal placement
- **Reconciliation Loops:** Continuously compares desired vs actual replica count; reschedules tasks on healthy nodes when failures are detected
- **Scale Up/Down:** Supports dynamic horizontal scaling of services

### 4. Control Plane API (REST)
HTTP-based API for cluster management and workload submission.

- **REST Communication:** HTTP/JSON APIs for manager and worker interactions
- **Request Forwarding:** Non-leader managers forward client requests to the elected leader
- **Worker APIs:** Workers continuously push heartbeats and live CPU/memory stats to managers
- **Observability:** Prometheus integration for tracking end-to-end application scheduling latency

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

### Consensus-Driven Operations
Every scheduling decision is committed to etcd before execution:

```
1. API Request → Control Plane
2. Proposal → Raft Leader
3. Replication → Raft Followers
4. Commit → Durable State
5. Execute → Worker Nodes
```

### Self-Healing & Reconciliation
ORION continuously reconciles desired vs. actual state:
- **Desired State:** What should be running (persisted in etcd)
- **Actual State:** What is running (from worker heartbeats)
- **Reconciliation:** Automatic correction of drift — failed tasks are rescheduled on healthy workers

### Failure Resilience
- **Node Failures:** Automatic workload rescheduling via reconciliation loop
- **Leader Failures:** New leader elected via Raft quorum
- **Task Crashes:** Detected via missing heartbeats; tasks reassigned automatically

---

## Installation

### Prerequisites
- Go 1.21 or higher
- Docker
- etcd

### Build from Source

```bash
# Clone the repository
git clone https://github.com/palpatel224/Orion.git
cd Orion

# Install dependencies
go mod download

# Build the orchestrator
go build -o orion ./cmd/pkg/

# Build the CLI
go build -o orionctl ./cmd/orionctl/
```

---

## Quick Start

### 1. Start etcd

```bash
etcd
```

### 2. Start the Orchestrator

```bash
./orion
```

This starts 3 manager nodes (ports 8080, 8081, 8082) and 3 worker nodes (ports 5555, 5556, 5557), waits for leader election, registers workers, and submits the demo application.

### 3. Get Running Tasks

```bash
orionctl get tasks --manager-addr http://localhost:8080
```

---

## Testing

Tested by running multiple manager nodes (ports 8080, 8081, 8082) and worker nodes (ports 5555, 5556, 5557) on the same host, and also across two physical nodes with managers on one and workers on the other.

---

## Monitoring & Observability

ORION exposes Prometheus-compatible metrics:
- End-to-end application scheduling latency
---

## Roadmap

- [x] **v0.1** - Core Raft implementation with etcd
- [x] **v0.2** - Network monitoring and scoring
- [x] **v0.3** - Network-aware scheduler with dependency resolution
- [ ] **v0.4** - YAML-based application manifest and CLI submission
- [ ] **v0.5** - Multi-cluster support

---

## Acknowledgments

- The Raft consensus algorithm by Diego Ongaro and John Ousterhout
- The Kubernetes community for orchestration patterns
- Docker for container runtime

---

## Contact

**Pal Patel** - 231CS240  
**Aditi Pandey** - 231IT003

Project Link: [https://github.com/palpatel224/Orion](https://github.com/palpatel224/Orion)
