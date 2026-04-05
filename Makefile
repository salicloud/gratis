GOPATH := $(shell go env GOPATH)
PATH   := $(PATH):$(GOPATH)/bin
PROTO_SRC := proto
GEN_OUT   := gen

.PHONY: proto agent api test lint clean

proto:
	protoc \
		--proto_path=$(PROTO_SRC) \
		--go_out=$(GEN_OUT) \
		--go_opt=paths=source_relative \
		--go-grpc_out=$(GEN_OUT) \
		--go-grpc_opt=paths=source_relative \
		$(shell find $(PROTO_SRC) -name '*.proto')

agent:
	go build -o bin/gratis-agent ./agent/cmd/agent

api:
	go build -o bin/gratis-api ./api/cmd/api

test:
	go test ./...

lint:
	golangci-lint run ./...

clean:
	rm -rf bin/ gen/
