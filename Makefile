
CURRENT_DIR := $(shell pwd)
export PATH := $(CURRENT_DIR)/tools/:$(PATH)

proto:
	protoc -I=./ --go_out=paths=source_relative:. ./actor/*.proto

.PHONY: proto