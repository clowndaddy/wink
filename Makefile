BINARY  := bin/wink
CMD     := ./cmd/wink
MODULE  := wink

.PHONY: all build clean test install

all: build

## build: compile the wink CLI into bin/wink
build:
	@mkdir -p bin
	go build -o $(BINARY) $(CMD)
	@echo "Built $(BINARY)"

## test: run all tests
test:
	go test ./... -v

## clean: remove the bin directory
clean:
	rm -rf bin

## install: install wink to GOPATH/bin (system-wide)
install:
	go install $(CMD)

## help: print this help
help:
	@grep -E '^##' Makefile | sed 's/## //'