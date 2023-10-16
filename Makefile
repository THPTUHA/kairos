compile-proto:
	@echo
	@echo "==> Compiling Protobuf files <=="
	protoc  --go_out=server/plugin --go_opt=paths=source_relative --go-grpc_out=server/plugin --go-grpc_opt=paths=source_relative  proto/*.proto