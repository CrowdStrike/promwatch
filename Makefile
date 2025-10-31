VERSION = 0.8.1
GIT_HASH = $(shell git rev-parse --short HEAD)
DATE = $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS = -X main.Version=$(VERSION) \
          -X main.GitHash=$(GIT_HASH) \
          -X main.Date=$(DATE)

.PHONY: test lint cover package
all: test lint promwatch

test:
	go test -race ./...

lint:
	go vet ./...
	golangci-lint run

cover.out: $(wildcard *.go)
	go test -coverprofile=$@ ./...

cover: cover.out
	go tool cover -html=$<

promwatch: $(wildcard *.go) go.mod go.sum
	go build -o $@ -ldflags="$(LDFLAGS)"
