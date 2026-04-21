PROTO_DIR   := proto
OUT_DIR     := proto
PROTOC_OPTS := --go_out=$(OUT_DIR) --go_opt=paths=source_relative \
               --go-grpc_out=$(OUT_DIR) --go-grpc_opt=paths=source_relative

.PHONY: proto build run-routing run-pop all

all: proto build

proto:
	@echo "→ Generating Go gRPC stubs..."
	protoc -I$(PROTO_DIR) $(PROTOC_OPTS) $(PROTO_DIR)/origin.proto

build:
	@echo "→ Building routing_server..."
	go build -o bin/routing_server ./routing_server
	@echo "→ Building pop_server..."
	go build -o bin/pop_server ./pop_server
	@echo "✓ Binaries in bin/"

run-routing:
	./bin/routing_server -addr :9200 -ttl 60

run-pop:
	./bin/pop_server \
		-rtmp :1935 \
		-http :8080 \
		-routing localhost:9200 \
		-id pop-1 \
		-region us-west

deps:
	go mod tidy