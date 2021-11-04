# This Makefile is meant to be used by people that do not usually work
# with Go source code. If you know what GOPATH is then you probably
# don't need to bother with make.

node_server:
	@echo "Node building."
	go build -o ./bin/node node/main.go node/nodecli.go

all:
	node_server
