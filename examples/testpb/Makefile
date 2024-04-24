
CURRENT_DIR := $(shell pwd)
export PATH := $(CURRENT_DIR)/../../tools/:$(PATH)

proto:
	echo "$(CURRENT_DIR)"
	protoc -I=./ --go_out=paths=source_relative:./ ./*.proto

.PHONY: proto