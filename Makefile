.PHONY: build test lint fmt vet

build:
	go build ./...

test:
	go test -race ./...

vet:
	go vet ./...

fmt:
	gofmt -l .

lint: fmt vet
