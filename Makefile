SHELL := /bin/bash

.PHONY: build-cli up-etcd down-etcd reset-etcd start clean test-cli test-cli-run

build-cli:
	go build -o orionctl ./cmd/orionctl

up-etcd:
	docker compose up -d etcd

down-etcd:
	docker compose down

reset-etcd: up-etcd
	etcdctl --endpoints=localhost:12379 del --prefix "/"

start:
	bash ./scripts/start.sh

clean:
	bash ./scripts/cleanup.sh

test-cli: build-cli
	./orionctl get tasks --manager-addr http://localhost:8080
	./orionctl get nodes --manager-addr http://localhost:8080

test-cli-run: build-cli
	./orionctl run -f task.json --manager-addr http://localhost:8080
	./orionctl get tasks --manager-addr http://localhost:8080
	./orionctl get nodes --manager-addr http://localhost:8080
