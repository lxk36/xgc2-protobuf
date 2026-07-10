# XGC2 Protocols

Cross-language protocol source of truth for XGC2 Core, robot adapters,
simulators, and Python tools. ROS1 is a consumer of these contracts, not the
owner of the shared protocol.

## Prototype shape

```text
proto/xgc/v1/            Stable, direction-neutral Message
proto/xgc/adapter/v1/    AdapterLink gRPC lifecycle and data flows
proto/xgc/semantic/      Extensible XGC2 semantic payload messages
registry/                Global message ID registry
profiles/                Native Adapter mappings such as PX4/MAVROS
tools/                   Reproducible generation and validation
tests/                   C++, Go, and Python round-trip smoke tests
generated/               Generated artifacts; ignored by Git
```

The `xgc.v1.Message` envelope carries a `message_id` and protobuf-encoded
`bytes payload`.
Generated registries map that ID back to a concrete C++, Go, or Python message
class. Adding a semantic capability extends the semantic catalog and registry;
it does not change `xgc.v1.Message` or the Adapter gRPC service.

See [docs/architecture.md](docs/architecture.md) for the current design
boundary and [profiles/ros1/px4-mavros-v1.yaml](profiles/ros1/px4-mavros-v1.yaml)
for a concrete PX4 example.

## Generate

Required runtime tools:

- `protoc` and the Protobuf C++ development package
- `grpc_cpp_plugin`
- Python `grpcio-tools`, `protobuf`, and `PyYAML`
- pinned `protoc-gen-go` and `protoc-gen-go-grpc`

The Go plugins are installed into the repository-local ignored tool directory:

```bash
tools/install-go-plugins.sh
```

Generate all three language outputs:

```bash
tools/generate.sh
```

Generated output:

```text
generated/cpp/
generated/go/
generated/python/
generated/descriptors/
generated/registry.json
```

## Verify

```bash
tools/smoke-test.sh
```

The smoke test validates both Adapter profiles and performs the same semantic
payload round trip in C++, Go, and Python:

```text
FlightModeRequest("OFFBOARD")
  -> protobuf bytes
  -> xgc.v1.Message(message_id=3111)
  -> generated registry
  -> FlightModeRequest("OFFBOARD")
```

It also runs an in-memory Go gRPC integration test covering Adapter
registration, telemetry upload, streamed operations, and operation-result
reporting.

The current host has an incomplete `/usr/local` gRPC C++ development install,
so the smoke test compiles the C++ Wire/payload/registry path and separately
generates the gRPC C++ stubs. A Debian build image must provide a consistent
gRPC C++ development package before linking the full Adapter service.

## Planned packages

- C++ headers and libraries through Debian packages
- Go generated module through a local Debian package
- Python generated package through a wheel and/or Debian package

TypeScript and other languages are outside the first phase.

## Integration status

This repository currently runs source generation and cross-language tests
only. It intentionally has no `.xgc2/product.yml`, release workflow, Debian
publication job, or `xgc2-devops` catalog entry while the protocol is being
designed.
