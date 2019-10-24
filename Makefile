.PHONY: all build clean plugin

PLUGIN_NAME = brindster/docker-plugin-cephfs
PLUGIN_TAG ?= master

all: clean build plugin

clean:
	rm -rf ./build

build:
	docker build --quiet --tag ${PLUGIN_NAME}:rfs .
	mkdir -p build/rootfs
	docker create --name tmp ${PLUGIN_NAME}:rfs true
	docker export tmp | tar -x -C build/rootfs
	cp config.json ./build/
	docker rm -vf tmp

plugin:
	docker plugin rm --force ${PLUGIN_NAME}:${PLUGIN_TAG} || true
	docker plugin create ${PLUGIN_NAME}:${PLUGIN_TAG} build
