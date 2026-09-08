BINARY := kubectl-env_diff
VERSION ?= $(shell git describe --tags --always --dirty)

.PHONY: build test lint cover clean

build:
	go build -ldflags "-X main.version=$(VERSION)" -o $(BINARY) ./cmd/kubectl-env_diff

test:
	go test ./... -race -count=1

cover:
	go test ./... -coverprofile=coverage.out -covermode=atomic
	go tool cover -func=coverage.out | tail -1

lint:
	go vet ./...
	gofmt -l . | tee /dev/stderr | (! read)

clean:
	rm -f $(BINARY) coverage.out
