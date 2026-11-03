# The image is named after this folder, which is the repo's name.
IMAGE := $(notdir $(CURDIR))

.PHONY: run worker test test-integration test-nats lint build

run:
	go run ./cmd/app serve

worker:
	go run ./cmd/app worker

test:
	go test -race ./...

# Core's provider-route scenarios against the service, over Postgres in Docker.
# CORE_MIGRATIONS is core's backend/migrations directory.
test-integration:
	CORE_MIGRATIONS=$(CORE_MIGRATIONS) go test -count=1 ./internal/server/integration

# The LMS job flow over a real NATS server in Docker: the publisher, the queue
# group and the worker's handlers.
test-nats:
	docker run -d --rm --name pp-test-nats -p 127.0.0.1:14222:4222 nats:2 >/dev/null
	NATS_TEST_URL=nats://127.0.0.1:14222 go test -race -count=1 -run 'OverNATS|OneWorker' ./internal/providerplatforms; \
	status=$$?; docker stop pp-test-nats >/dev/null; exit $$status

# The same golangci-lint version CI runs, without installing it locally.
lint:
	docker run --rm -v $(CURDIR):/app -w /app golangci/golangci-lint:v2.14.0 golangci-lint run

build:
	docker build -t $(IMAGE) .
