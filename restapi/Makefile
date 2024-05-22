GOFLAGS=-ldflags=-w -ldflags=-s
LOCAL_BUILD_PATH=./bin

## Rest Api
build-restapi:
	@echo "Building restapi binary"
	CGO_ENABLED=0 go build $(GOFLAGS) -o $(LOCAL_BUILD_PATH)/restapi .