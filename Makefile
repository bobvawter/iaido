.PHONY: build clean generate fmt install lint test

all: release

build:
	go build -ldflags "-s -w -X github.com/bobvawter/iaido/pkg/frontend.buildID=`git describe --tags --always --dirty`" -o bin/iaido ./cmd/iaido
	tar cfz bin/iaido$(BUILD_SUFFIX).tar.gz -C bin iaido

clean:
	go clean ./... 

generate: 
	go generate ./... 

fmt:
	go fmt ./... 

install:
	go install 

lint: generate
	go run golang.org/x/lint/golint -set_exit_status ./...
	go run honnef.co/go/tools/cmd/staticcheck -checks all ./...

test: generate
	go vet ./...
	go test -v -race -coverprofile=coverage.txt ./...
	find configs -name *.yaml -print0 | xargs -0 -L1 -t go run github.com/bobvawter/iaido/cmd/iaido --check -c

release: fmt lint test build

