.PHONY: proto
proto:
	protoc \
		--proto_path=api/v1 \
		--go_out=gen/proto/v1 \
		--go_opt=paths=source_relative \
		--go-grpc_out=gen/proto/v1 \
		--go-grpc_opt=paths=source_relative \
		api/v1/odyssey.proto
