default: all
GOFLAGS=-ldflags=-w -ldflags=-s
SYNTEST_PLUGIN_SUBDIRS = $(notdir $(wildcard ./agent/plugins/syntests/*))

LOCAL_BUILD_PATH=./bin

## Proto
build-go-proto:
	@echo Building Golang proto files...
	protoc --go_out=plugins=grpc:common/proto/  common/proto/syntest.proto --proto_path=common/proto/

## Agent
build-go-syntest-plugins:
	@echo "Building Go syntest plugins"
	cd agent && \
	for dir in $(SYNTEST_PLUGIN_SUBDIRS); do \
			echo "Building plugin: $$dir" && \
			CGO_ENABLED=0 go build $(GOFLAGS) -o $(LOCAL_BUILD_PATH)/plugins/test-$$dir ./plugins/syntests/$$dir/ && \
			echo "$$dir compiled!" || { echo "$$dir failed!"; exit 1; }; \
	done

build-agent-only:
	@echo "Building agent"
	cd agent && CGO_ENABLED=0 go build $(GOFLAGS) -o $(LOCAL_BUILD_PATH)/agent .

.PHONY: build-agent
build-agent: build-agent-only build-go-syntest-plugins

docker-agent:
	@echo "Building agent container image"
	cd agent && podman build -f Dockerfile -t synheart-agent:dev_latest ..

## Rest Api
build-restapi:
	@echo "Building restapi binary"
	cd restapi && CGO_ENABLED=0 go build $(GOFLAGS) -o $(LOCAL_BUILD_PATH)/restapi .

.PHONY: docker-restapi
docker-restapi:
	@echo "Building restapi container image"
	cd restapi && podman build -f Dockerfile -t synheart-restapi:dev_latest ..

## Controller
.PHONY: docker-controller
docker-controller:
	@echo "Building controller container image"
	cd controller && podman build -f Dockerfile -t synheart-controller:dev_latest ..

.PHONY : docker-all
docker-all: docker-agent docker-restapi docker-controller

#.PHONY : test
#test: build-dummy-test-plugins build-syn-heart
#	go test -race -v synthetic-heart/agent
#	go test -race -v synthetic-heart/broadcaster


## SDK - Creating new syntest plugin
.PHONY : new-go-test
new-go-test:
	@echo Creating Directory plugins/syntests/$(name)
	mkdir agent/plugins/syntests/$(name)

	@echo Creating file plugins/syntests/$(name)/$(name).go
	cp agent/plugins/templates/go/test.tpl.go plugins/syntests/$(name)/$(name).go

	sed -i .backup s/{{TestName}}/$(call capitalize,$(name))/g  plugins/syntests/$(name)/$(name).go
	rm agent/plugins/syntests/$(name)/$(name).go.backup


.PHONY : clean
clean:
	rm -rf agent/bin
	rm -rf controller/bin
	rm -rf restapi/bin

define capitalize
$(shell echo $(1) | awk '{$$1=toupper(substr($$1,0,1))substr($$1,2); print $$0}')
endef
