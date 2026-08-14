.PHONY: test test-release build install

test:
	go test ./...
	bash scripts/release-tag_test.sh

test-release:
	bash scripts/release-tag_test.sh

build:
	go build -o bin/champu ./cmd/champu

install:
	go install ./cmd/champu
