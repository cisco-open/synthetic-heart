GOFLAGS=-ldflags=-w -ldflags=-s
SYNTEST_PLUGIN_SUBDIRS = $(notdir $(wildcard ./plugins/syntests/*))
LOCAL_BUILD_PATH=./bin

## Agent
build-go-syntest-plugins:
	@echo "Building Go syntest plugins"
	for dir in $(SYNTEST_PLUGIN_SUBDIRS); do \
			echo "Building plugin: $$dir" && \
			CGO_ENABLED=0 go build $(GOFLAGS) -o $(LOCAL_BUILD_PATH)/plugins/test-$$dir ./plugins/syntests/$$dir/ && \
			echo "$$dir compiled!" || { echo "$$dir failed!"; exit 1; }; \
	done

build-agent-only:
	@echo "Building agent"
	CGO_ENABLED=0 go build $(GOFLAGS) -o $(LOCAL_BUILD_PATH)/agent .

.PHONY: build-agent
build-agent: build-agent-only build-go-syntest-plugins

## SDK - Creating new syntest plugin
.PHONY : new-go-syntest
new-go-syntest:
	@echo Creating Directory plugins/syntests/$(name)
	mkdir plugins/syntests/$(name)

	@echo Creating file plugins/syntests/$(name)/$(name).go
	cp plugins/templates/go/test.tpl.go plugins/syntests/$(name)/$(name).go

	sed -i s/{{TestNameRaw}}/$(name)/g  plugins/syntests/$(name)/$(name).go
	sed -i s/{{TestName}}/$(call capitalize,$(name))/g  plugins/syntests/$(name)/$(name).go

define capitalize
$(shell echo $(1) | awk '{$$1=toupper(substr($$1,0,1))substr($$1,2); print $$0}')
endef
