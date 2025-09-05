
CURRENT_DIR := $(shell pwd)
export PATH := $(CURRENT_DIR)/tools/:$(PATH)

proto:
	protoc --version
#	protoc -I=./proto/internal --go_out=paths=source_relative:./internal ./proto/internal/*.proto
#	protoc -I=./proto --go_out=paths=source_relative:. ./proto/*.proto
#	protoc -I=./proto --go-grpc_out=paths=source_relative:. ./proto/*.proto
	protoc --go_out=paths=source_relative:. ./message/msg.proto
	protoc --go_out=paths=source_relative:. ./remote/remote.proto
	protoc --go-grpc_out=paths=source_relative:. ./remote/remote.proto

.PHONY: proto