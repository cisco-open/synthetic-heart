GOFLAGS=-ldflags=-w -ldflags=-s
SYNTEST_PLUGIN_SUBDIRS = $(notdir $(wildcard ./plugins/syntests/*))
LOCAL_BUILD_PATH=./bin
SYNHEART_VERSION=v1.2.1

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
	CGO_ENABLED=0 go build $(GOFLAGS) -ldflags="-X main.Version=$(SYNHEART_VERSION)" -o $(LOCAL_BUILD_PATH)/agent .

.PHONY: build-agent
build-agent: build-agent-only build-go-syntest-plugins

## SDK - Creating new syntest plugin
.PHONY : new-go-syntest
new-go-syntest:
	@echo "-> Creating Directory plugins/syntests/$(name)"
	mkdir plugins/syntests/$(name)

	@echo "-> Creating file plugins/syntests/$(name)/$(name).go"
	cp plugins/templates/go/test.tpl.go plugins/syntests/$(name)/$(name).go

	@echo "-> Replacing placeholders in $(name).go"
	sed -i s/{{TestNameRaw}}/$(name)/g  plugins/syntests/$(name)/$(name).go
	sed -i s/{{TestName}}/$(call capitalize,$(name))/g  plugins/syntests/$(name)/$(name).go

.PHONY : new-python-syntest
new-python-syntest:
	@echo "-> Creating Directory plugins/syntests-python/$(name)"
	mkdir -p plugins/syntests-python/$(name)

	@echo "-> Creating file plugins/syntests/$(name)/$(name).py"
	cp plugins/templates/python/test.tpl.py plugins/syntests-python/$(name)/test-$(name).py
	cp plugins/templates/python/requirements.txt plugins/syntests-python/$(name)/requirements.txt

	@echo "-> Replacing placeholders in test-$(name).py"
	sed -i.bak s/{{TestName}}/$(call camelcase,$(name))/g plugins/syntests-python/$(name)/test-$(name).py && rm plugins/syntests-python/$(name)/test-$(name).py.bak

	@echo "-> Creating virtual environment"
	python3 -m venv plugins/syntests-python/$(name)/.venv

	@echo "\nActivate virtual environment by running:\n \n source plugins/syntests-python/$(name)/.venv/bin/activate"

define capitalize
$(shell echo $(1) | awk '{$$1=toupper(substr($$1,0,1))substr($$1,2); print $$0}')
endef

define camelcase
$(shell echo $(1) | awk 'BEGIN{FS="";RS="-";ORS=""} {$$0=toupper(substr($$0,1,1)) substr($$0,2)} 1')
endef
