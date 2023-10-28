compile-proto:
	@echo
	@echo "==> Compiling Protobuf files <=="
	protoc  --go_out=server/plugin --go_opt=paths=source_relative --go-grpc_out=server/plugin --go-grpc_opt=paths=source_relative  proto/*.proto

plugin:
	cd server/plugin/kairos-executor-http && go build . -o /
agent:
	@echo "==> Create agent <=="
	rm kairos 
	go build .
	./kairos  agent --server --bootstrap-expect=1
migrate-create:
	migrate create -ext sql -dir server/storage/migration/ -seq kairos
migrate-up:
	migrate -path server/storage/migration -database postgresql://kairos:kairos@localhost:5432/kairos?sslmode=disable up
migrate-force:
	migrate -path server/storage/migration -database postgresql://kairos:kairos@localhost:5432/kairos?sslmode=disable force ${v}