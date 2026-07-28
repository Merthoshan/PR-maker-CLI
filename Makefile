.PHONY: test build install

test:
	go test ./...

build:
	go build -o bin/champu-pr ./cmd/champu-pr

install:
	go install ./cmd/champu-pr
