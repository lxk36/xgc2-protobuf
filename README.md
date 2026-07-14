# XGC2 Protocols

Cross-language protocol source of truth for XGC2 Core, robot adapters,
simulators, and Python tools. ROS1 is a consumer of these contracts, not the
owner of the shared protocol.

## Contract shape

```text
proto/xgc/v1/            Stable, direction-neutral Message
proto/xgc/adapter/v1/    AdapterLink gRPC lifecycle and data flows
proto/xgc/semantic/      Extensible XGC2 semantic payload messages
registry/                Global message ID registry
profiles/schema/         Formal Adapter profile JSON Schema
profiles/ros1/           Native Adapter mappings for supported robots
tools/                   Reproducible generation and validation
tests/                   C++, Go, and Python round-trip smoke tests
generated/               Generated artifacts; ignored by Git
```

The `xgc.v1.Message` envelope carries a `message_id` and protobuf-encoded
`bytes payload`.
Generated registries map that ID back to a concrete C++, Go, or Python message
class. Adding a semantic capability extends the payload catalog and registry;
it does not change `xgc.v1.Message` or the Adapter gRPC service.

See [docs/architecture.md](docs/architecture.md) for the design boundary,
[docs/profile-contract.md](docs/profile-contract.md) for the profile digest
contract, and
[profiles/ros1/px4-multirotor-ros1-v2.yaml](profiles/ros1/px4-multirotor-ros1-v2.yaml)
for the PX4 profile.

## Generate

Required runtime tools:

- `protoc` and the Protobuf C++ development package
- `grpc_cpp_plugin`
- Python `grpcio-tools`, `protobuf`, `jsonschema`, and `PyYAML`
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
generated/profile-registry.json
```

## Verify

```bash
tools/smoke-test.sh
```

The smoke test validates both Adapter profiles and performs the same payload
round trip in C++, Go, and Python:

```text
ModeRequest(mode="OFFBOARD")
  -> protobuf bytes
  -> xgc.v1.Message(message_id=3202)
  -> generated registry
  -> ModeRequest(mode="OFFBOARD")
```

It also runs an in-memory Go gRPC integration test covering Adapter
registration, telemetry upload, streamed operations, and operation-result
reporting.

CI runs `buf lint` for every change. Pull requests also run `buf breaking`
against the target branch when both versions are in the same protocol epoch.
Before 1.0, a minor-version change starts a deliberate breaking epoch; from
1.0 onward, only a major-version change does so. Version `0.3.0-1` starts a
deliberate breaking epoch: registry and protobuf encoding are negotiated once
per Adapter session instead of repeated in every message, and PX4 profile v2
adds the mocap and Offboard diagnostic contract.

The public contract exposes semantic arm, mode, and normal autopilot-reboot
requests. Raw MAVLink commands, ROS topic names, ROS service names, and native
message types are profile implementation details and are never accepted from an
individual Core operation.

The current host has an incomplete `/usr/local` gRPC C++ development install,
so the smoke test compiles the C++ Wire/payload/registry path and separately
generates the gRPC C++ stubs. A Debian build image must provide a consistent
gRPC C++ development package before linking the full Adapter service.

## Debian schema development package

The first published product is `xgc2-protobuf-dev` (`Architecture: all`) for
Ubuntu Focal, Jammy, and Noble. It provides the stable, language-neutral inputs
needed by protocol consumers:

- `.proto` source files under `/usr/share/xgc2-protobuf/proto`;
- the complete descriptor set under `/usr/share/xgc2-protobuf/descriptors`;
- the source YAML and generated JSON message registries;
- the formal Adapter profile schema, source profiles, generated digest
  registry, and protocol design documentation;
- `find_package(xgc2_protobuf CONFIG)` and `pkg-config xgc2-protobuf`
  discovery metadata.

Install and inspect the schema root with:

```bash
sudo apt update
sudo apt install xgc2-protobuf-dev
pkg-config --variable=proto_root xgc2-protobuf
```

The Debian package deliberately does **not** install generated C++, Go, or
Python bindings. Each consumer generates bindings into its own build tree with
its pinned toolchain. This prevents a schema package from silently selecting a
distro-specific protobuf ABI, Python namespace, or Go module version.

## Conflict and migration boundary

`xgc2-protobuf-dev` coexists with Ubuntu's protobuf compiler and development
packages. It owns only `/usr/share/xgc2-protobuf`, its CMake config directory,
and its pkg-config file; it does not install files into `/usr/include`, Python
site-packages, the Go module cache, or a ROS prefix.

Consumers migrating from vendored schemas should depend on
`xgc2-protobuf-dev`, discover `XGC2_PROTOBUF_PROTO_ROOT` (or the corresponding
pkg-config variable), and generate bindings in a private build directory.
Existing generated runtime packages must not be overwritten or claimed by this
package. A future language-specific runtime package requires a separate name,
ownership boundary, and compatibility policy.

## Integration status

This repository is an XGC2 `toolchain-apt` product with product id
`xgc2-protobuf`. Push CI keeps the existing C++, Go, and Python generation tests
and runs package compliance and Focal/Jammy/Noble Debian builds in parallel.
Each successful distro build emits one schema-only deb plus independent amd64
and arm64 trusted build manifests retained for 14 days. The fallback release
workflow prepares the same artifacts for the centralized `xgc2-devops` release
train and never receives APT publication credentials.
