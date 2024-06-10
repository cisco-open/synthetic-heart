build-go-proto:
	@echo Building Golang proto files...
	protoc --go-grpc_out=.  proto/syntest.proto --proto_path=proto/
	protoc --go_out=.  proto/syntest.proto --proto_path=proto/

build-python-proto:
	@echo Building Python proto files...
	protoc --python_out=.  proto/syntest.proto --proto_path=proto/