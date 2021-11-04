# This Makefile is meant to be used by people that do not usually work
# with Go source code. If you know what GOPATH is then you probably
# don't need to bother with make.
BINARY_NODE=node
GOFILES=$(widcard *.go)

build_node:
	@echo "Node building."
	go build -o ./bin/node node/main.go node/nodecli.go

#build:
#	go build -o ${BINARY_NAME} -ldflags "-s -w" ${GOFILES}

## run: run this program
#run:
#	go run ${GOFILES}


## fmt: format go files
fmt:
	go fmt ./...

## clean: clean
clean:
	go clean 
	rm -f bin/$(BINARY_NODE)

all:
	build_node


.PHONY: help run build fmt clean