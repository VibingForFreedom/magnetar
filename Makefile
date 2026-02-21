GO_TAGS := sqlite_fts5
GO_FLAGS := -tags "$(GO_TAGS)"

.PHONY: build test test-race test-mariadb vet clean run frontend dev-frontend

build: frontend
	go build $(GO_FLAGS) ./...

build-go:
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
	rm -rf frontend/build frontend/.svelte-kit frontend/node_modules

frontend:
	cd frontend && npm ci && npm run build
	rm -rf internal/web/static/_app internal/web/static/robots.txt
	cp -r frontend/build/* internal/web/static/

dev-frontend:
	cd frontend && npm run dev

.DEFAULT_GOAL := build
