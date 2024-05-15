build-go-proto:
	@echo Building Golang proto files...
	protoc --go-grpc_out=.  proto/syntest.proto --proto_path=proto/
	protoc --go_out=.  proto/syntest.proto --proto_path=proto/