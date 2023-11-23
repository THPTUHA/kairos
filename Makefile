
GOPATH=$(shell go env GOPATH)
ROOT_DIR:=$(shell dirname $(realpath $(firstword $(MAKEFILE_LIST))))

compile-proto:
	@echo
	@echo "==> Compiling Protobuf files <=="
	protoc  --go_out=server/plugin  \
			--go_opt=paths=source_relative \
			--go-grpc_out=server/plugin \
			--go-grpc_opt=paths=source_relative  proto/*.proto

	 

plugin:
	cd server/plugin/kairos-executor-http && go build  -o  ${ROOT_DIR}/kairos-local/kairosdeamon
	cd server/plugin/kairos-executor-nats && go build  -o  ${ROOT_DIR}/kairos-local/kairosdeamon
	cd server/plugin/kairos-executor-file && go build  -o  ${ROOT_DIR}/kairos-local/kairosdeamon

agent:
	@echo "==> Create agent <=="
	rm kairos 
	go build .
	./kairos  agent --server --bootstrap-expect=1

migrate-create:
	migrate create -ext sql -dir server/storage/migration/ -seq kairos
resetdb:
	psql -U kairos -d kairos -a -f server/storage/script/reset.sql 
migrate-up:
	migrate -path server/storage/migration -database postgresql://kairos:kairos@localhost:5432/kairos?sslmode=disable up

migrate-down:
	migrate -path server/storage/migration -database postgresql://kairos:kairos@localhost:5432/kairos?sslmode=disable down

migrate-force:
	migrate -path server/storage/migration -database postgresql://kairos:kairos@localhost:5432/kairos?sslmode=disable force ${v}