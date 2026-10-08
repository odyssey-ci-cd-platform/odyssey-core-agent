---
description: Regenerate gRPC/protobuf code from api/v1/odyssey.proto
---

Run `make proto` to regenerate the gRPC/protobuf code from `api/v1/odyssey.proto` into `gen/proto/v1`.

Requires `protoc`, `protoc-gen-go`, and `protoc-gen-go-grpc` on `PATH`. If any are missing, tell the user which one and stop — don't try to install toolchains silently.

After regenerating, run `go build ./...` to confirm the generated code compiles, and `git diff --stat gen/` to show what changed.
