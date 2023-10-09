compile-proto:
	@echo
	@echo "==> Compiling Protobuf files <=="
	protoc --go_out=. --go_opt=paths=source_relative storage/sstables/proto/sstable.proto