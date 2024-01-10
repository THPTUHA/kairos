
GOPATH=$(shell go env GOPATH)
ROOT_DIR:=$(shell dirname $(realpath $(firstword $(MAKEFILE_LIST))))
.PHONY: start_nats

compile-proto:
	@echo
	@echo "==> Compiling Protobuf files <=="
	protoc  --go_out=server/plugin  \
			--go_opt=paths=source_relative \
			--go-grpc_out=server/plugin \
			--go-grpc_opt=paths=source_relative  proto/*.proto

	 

plugin:
	cd server/plugin/kairos-executor-mail && go build  -o  ${ROOT_DIR}/kairos-local/kairosdeamon
	cd server/plugin/kairos-executor-k8sdeploy && go build  -o  ${ROOT_DIR}/kairos-local/kairosdeamon
cc:
	cd server/plugin/kairos-executor-http && go build  -o  ${ROOT_DIR}/kairos-local/kairosdeamon
	cd server/plugin/kairos-executor-nats && go build  -o  ${ROOT_DIR}/kairos-local/kairosdeamon
	cd server/plugin/kairos-executor-simple && go build  -o  ${ROOT_DIR}/kairos-local/
	cd server/plugin/kairos-executor-file && go build  -o  ${ROOT_DIR}/kairos-local/kairosdeamon
	cd server/plugin/kairos-executor-sql && go build  -o  ${ROOT_DIR}/kairos-local/kairosdeamon
	cd server/plugin/kairos-executor-dockerlite && go build  -o  ${ROOT_DIR}/kairos-local/kairosdeamon
	cd server/plugin/kairos-executor-k8sdeploy && go build  -o  ${ROOT_DIR}/kairos-local/kairosdeamon

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

install_postgres:
	sudo yum install -y postgresql-server postgresql-contrib
	sudo postgresql-setup initdb
	sudo systemctl start postgresql
	sudo systemctl enable postgresql

create_database:
	sudo -u postgres psql -c "CREATE DATABASE kairos;"
	sudo -u postgres psql -c "CREATE USER kairos WITH PASSWORD 'kairos';"
	sudo -u postgres psql -c "ALTER ROLE kairos SET client_encoding TO 'utf8';"
	sudo -u postgres psql -c "ALTER ROLE kairos SET default_transaction_isolation TO 'read committed';"
	sudo -u postgres psql -c "ALTER ROLE kairos SET timezone TO 'UTC';"
	sudo -u postgres psql -c "GRANT ALL PRIVILEGES ON DATABASE kairos TO kairos;"

setupdb: install_postgres create_database
	psql -h localhost -U kairos -d kairos -f server/storage/migration/000001_kairos.up.sql

start_nats:
	@if [ $$(docker ps -q -f name=nats) ]; then \
		echo "Container 'nats-container' is already running."; \
	else \
		echo "Starting NATS container..."; \
		docker run -p 4222:4222 -d --name nats nats:latest; \
	fi

deploy:start_nats
	docker compose -f server/storage/docker-compose.yaml up -d
	docker compose -f server/httpserver/docker-compose.yml up -d
	docker compose -f server/deliverer/server/docker-compose.yml up -d

clean-cache:
	dscacheutil -flushcache

webui:
	cd ./ui && docker buildx build  --platform linux/amd64 -t nexta2020/webui .
	docker push nexta2020/webui

image:
	docker buildx build  --platform linux/amd64 -t nexta2020/kairos --load .
	docker push nexta2020/kairos

seed:
	psql -h 103.173.254.32 -U kairos -d kairos -f diagram.sql
query:
	psql -U kairos -d kairos -f diagram.sql

linux:
	cd server/plugin/kairos-executor-dockerlite && GOOS=linux GOARCH=amd64 go build  -o  ${ROOT_DIR}/release/linux
	cd server/plugin/kairos-executor-k8sdeploy && GOOS=linux GOARCH=amd64  go build  -o  ${ROOT_DIR}/release/linux
	cd server/plugin/kairos-executor-mail && GOOS=linux GOARCH=amd64  go build  -o  ${ROOT_DIR}/release/linux
	cd server/plugin/kairos-executor-http && GOOS=linux GOARCH=amd64  go build  -o  ${ROOT_DIR}/release/linux
	cd server/plugin/kairos-executor-nats && GOOS=linux GOARCH=amd64  go build  -o  ${ROOT_DIR}/release/linux
	cd server/plugin/kairos-executor-file && GOOS=linux GOARCH=amd64  go build  -o  ${ROOT_DIR}/release/linux
	cd server/plugin/kairos-executor-sql && GOOS=linux GOARCH=amd64  go build  -o  ${ROOT_DIR}/release/linux
	cd server/plugin/kairos-executor-script && GOOS=linux GOARCH=amd64  go build  -o  ${ROOT_DIR}/release/linux
	cd kairos-local/kairosdeamon && GOOS=linux GOARCH=amd64  go build  -o  ${ROOT_DIR}/release/linux
	cd kairos-local/kairosctl && GOOS=linux GOARCH=amd64  go build  -o  ${ROOT_DIR}/release/linux

window:
	cd server/plugin/kairos-executor-dockerlite && GOOS=windows GOARCH=amd64  go build  -o  ${ROOT_DIR}/release/window
	cd server/plugin/kairos-executor-k8sdeploy && GOOS=windows GOARCH=amd64   go build  -o  ${ROOT_DIR}/release/window
	cd server/plugin/kairos-executor-mail && GOOS=windows GOARCH=amd64   go build  -o  ${ROOT_DIR}/release/window
	cd server/plugin/kairos-executor-http && GOOS=windows GOARCH=amd64   go build  -o  ${ROOT_DIR}/release/window
	cd server/plugin/kairos-executor-nats && GOOS=windows GOARCH=amd64   go build  -o  ${ROOT_DIR}/release/window
	cd server/plugin/kairos-executor-file && GOOS=windows GOARCH=amd64   go build  -o  ${ROOT_DIR}/release/window
	cd server/plugin/kairos-executor-sql && GOOS=windows GOARCH=amd64   go build  -o  ${ROOT_DIR}/release/window
	cd server/plugin/kairos-executor-script && GOOS=windows GOARCH=amd64  go build  -o  ${ROOT_DIR}/release/window
	cd kairos-local/kairosdeamon && GOOS=windows GOARCH=amd64  go build  -o  ${ROOT_DIR}/release/window
	cd kairos-local/kairosctl && GOOS=windows GOARCH=amd64  go build  -o  ${ROOT_DIR}/release/window

release: linux window
