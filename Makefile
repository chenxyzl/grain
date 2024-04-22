
CURRENT_DIR := $(shell pwd)
export PATH := $(CURRENT_DIR)/tools/:$(PATH)

proto:
	protoc -I=./proto/internal --go_out=paths=source_relative:./actor/internal ./proto/internal/*.proto
	protoc -I=./proto --go_out=paths=source_relative:./actor ./proto/*.proto
	protoc -I=./proto --go-grpc_out=paths=source_relative:./actor ./proto/*.proto

.PHONY: proto