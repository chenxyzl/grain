
CURRENT_DIR := $(shell pwd)
export PATH := $(CURRENT_DIR)/tools/:$(PATH)

proto:
	protoc -I=./proto --go_out=paths=source_relative:./actor ./proto/actor.proto
	protoc -I=./proto --go_out=paths=source_relative:./actor --go-grpc_out=paths=source_relative:./actor ./proto/rpc.proto

.PHONY: proto