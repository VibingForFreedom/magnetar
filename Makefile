GO_TAGS := sqlite_fts5
GO_FLAGS := -tags "$(GO_TAGS)"

.PHONY: build test vet clean

build:
	go build $(GO_FLAGS) ./...

test:
	go test $(GO_FLAGS) ./...

test-race:
	go test $(GO_FLAGS) -race ./...

vet:
	go vet $(GO_FLAGS) ./...

run:
	go run $(GO_FLAGS) ./cmd/magnetar serve

clean:
	go clean -cache

.DEFAULT_GOAL := build
