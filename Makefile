# The image is named after this folder, which is the repo's name.
IMAGE := $(notdir $(CURDIR))

.PHONY: run worker test test-integration lint build

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

# The same golangci-lint version CI runs, without installing it locally.
lint:
	docker run --rm -v $(CURDIR):/app -w /app golangci/golangci-lint:v2.14.0 golangci-lint run

build:
	docker build -t $(IMAGE) .
