.PHONY: build clean generate install lint test

all: build

build: generate
	CGO_ENABLED=0 go build -ldflags "-s -w -X github.com/bobvawter/iaido/pkg/frontend.buildID=`git describe --tags --always --dirty`" -o bin/iaido$(BIN_SUFFIX) ./cmd/iaido
	tar cfz bin/iaido$(BUILD_SUFFIX).tar.gz -C bin iaido$(BIN_SUFFIX)

clean:
	go clean ./... 

generate: 
	go generate ./... 

install:
	go install

lint: generate
	go fmt ./...
	go vet ./...
	go run golang.org/x/lint/golint -set_exit_status ./...
	go run honnef.co/go/tools/cmd/staticcheck -checks all ./...
	find configs -name iaido*.yaml -print0 | xargs -0 -L1 -t go run github.com/bobvawter/iaido/cmd/iaido --check -c

test: generate
	go test -coverpkg=./pkg/... -coverprofile=coverage.txt -covermode=atomic ./pkg/...

testrace: generate
	go test -race -coverpkg=./pkg/... -coverprofile=coverage.txt -covermode=atomic ./pkg/...

release: lint test build

