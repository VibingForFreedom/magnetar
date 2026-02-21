GO_TAGS := sqlite_fts5
GO_FLAGS := -tags "$(GO_TAGS)"

.PHONY: build test test-race test-mariadb vet clean run

build:
	go build $(GO_FLAGS) ./...

test:
	go test $(GO_FLAGS) ./...

test-race:
	go test $(GO_FLAGS) -race ./...

test-mariadb:
	TEST_MARIADB_DSN="magnetar:magnetar@tcp(127.0.0.1:3306)/magnetar?parseTime=true" go test -tags "$(GO_TAGS),mariadb" ./internal/store/...

vet:
	go vet $(GO_FLAGS) ./...

run:
	go run $(GO_FLAGS) ./cmd/magnetar serve

clean:
	go clean -cache

.DEFAULT_GOAL := build
