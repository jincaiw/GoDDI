# Replication protobuf generation

The source `replication.proto` is the unmodified contract from the development documentation package. Its SHA-256 is `96c8fd050b078b4333f54a75c724f44f87813e42ffea9c0f8ce689e787548097`. An intentional contract change requires coordinated protocol review and regeneration of all descriptor users.

Pinned tools: `protoc --version` = `libprotoc 36.2`; `protoc-gen-go --version` = `protoc-gen-go v1.36.11`; `protoc-gen-go-grpc --version` = `protoc-gen-go-grpc 1.5.1`. The generated header reports the same compiler as `v7.36.2` using protoc's plugin version fields. The generated Go package is `github.com/jasonwa/goddi/internal/gen/replica/v1` (package name `v1`). `M` options preserve the original proto bytes and supply the Go import path.

The module uses `google.golang.org/protobuf v1.36.11` and `google.golang.org/grpc v1.80.0`. The existing `github.com/pressly/goose/v3 v3.27.1` requires grpc v1.80.0 transitively: pinning grpc v1.71.0 with `go get` would downgrade goose. The higher selected version preserves the existing dependency and satisfies the generator's grpc minimum; it is not evidence of an operational replication service.

From the module root, with exactly the tools above already available on `PATH`:

```sh
protoc -I contracts \
  --go_out=. --go_opt=module=github.com/jasonwa/goddi \
  --go_opt=Mreplication.proto=github.com/jasonwa/goddi/internal/gen/replica/v1 \
  --go-grpc_out=. --go-grpc_opt=module=github.com/jasonwa/goddi \
  --go-grpc_opt=Mreplication.proto=github.com/jasonwa/goddi/internal/gen/replica/v1 \
  contracts/replication.proto
protoc -I contracts --descriptor_set_out=/tmp/goddi-replication.pbdesc contracts/replication.proto
cmp /tmp/goddi-replication.pbdesc internal/enterprise/pullbudget/replication.pbdesc
go test ./internal/gen/replica/v1/...
```

The descriptor without source info has SHA-256 `9bb3bcd6c43c6fd07d4eb229a7858da538cfbb996cfe556a2561bac80bacdcae`. The package test checks the source hash and compares the generated Go reflection descriptor byte for byte with the pullbudget descriptor; the `protoc`/`cmp` command above independently checks the source-derived descriptor. Tool pinning does not install or start a service, grant roles, or implement peer authentication.
