VERSION ?= $(shell git describe --tags --always)
BIN     ?= ./homard

all: homard homard.8

homard: go.mod go.sum *.go
	go build -ldflags '-X main.version=$(VERSION)'

homard.8: $(BIN) homard.h2m
	help2man --include=homard.h2m --no-info --section=8 $(BIN) -o $@

check:
	! gofmt -s -d . | grep ''
	go vet ./...
	go test -cover ./...

clean:
	rm -f homard homard.8

.PHONY: all check clean
