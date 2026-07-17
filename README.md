# XGC2 Protocols

Cross-language source of truth for the XGC2 Adapter Runtime Link, reusable
semantic payloads, domain envelopes, descriptors, and the global semantic
message registry.

## Contract shape

```text
proto/xgc/adapter/v1/   Capability-first Adapter Runtime Link
proto/xgc/v1/           Domain-neutral payload, schema, time, and message types
proto/xgc/robot/v1/     Robot-domain routing wrapper
proto/xgc/semantic/     Reusable semantic payload schemas
registry/               Global semantic message ID registry
tools/                  Reproducible generation and validation
tests/                  C++, Go, and Python contract tests
generated/              Ignored generated artifacts
```

The common product deliberately owns no native Adapter profiles, ROS endpoints,
robot inventory, run binding selection, or middleware conversion policy. Native
mapping
contracts belong to the concrete Adapter product that implements them.

## Adapter Runtime Link

`xgc.adapter.v1.AdapterRuntimeLinkService` has three RPCs:

- `Register` authenticates one prepared instance and process generation, proves
  the running build/manifest/SDK identity, and advertises immutable capability
  contracts;
- bidirectional `Control` carries revisioned `AdapterInstanceSpec`, heartbeat,
  atomic apply results, capability readiness, drain, and stop frames;
- bidirectional `Work` carries unary requests, durable operation events, and
  Host-initiated, credit-controlled source streams.

The Process Supervisor passes exactly one mode-0600 binary
`AdapterProcessBootstrap` file containing the Runtime target, registration
identity/proofs/contracts, and first complete instance spec. The SDK, Host, and
Supervisor therefore share one protobuf mapping instead of reconstructing
security-sensitive bootstrap fields in each Adapter.

The Runtime Host owns instance identity, scope, enabled capabilities, and
configuration. A running Adapter may report only the capability contracts
allowed by its trusted definition. Session frames are fenced by process
generation, session generation, Runtime epoch, connection epoch, and a
per-connection frame sequence. Readiness and every Work dispatch identify the
exact immutable capability contract by ID, version, and digest.
Control and Work reconnect as a pair and use the same `connection_epoch`.
The Adapter's first Work frame is always `WorkAttach`, allowing the Host to
identify the otherwise request-silent bidi stream before dispatch.

Runtime Link protocol v2 is the one exact supported Link contract; registration
does not negotiate a version set or retain a legacy fallback. It is deliberately
source-only. The Host allocates every source stream ID in
`SourceOpenRequest.context.work_id` and grants the first Adapter-to-Host credit.
The Adapter echoes that identity as `stream_id` in every response;
`accepted=true` represents
a successful empty open, while an error rejects it. Only the Adapter sends
`SourceData` and `SourceClose`, and only the Host sends `SourceCredit` and
an exact-oneof `WorkCancellation`. There is no Adapter-initiated open, sink, duplex, or
direction-neutral stream-frame compatibility path.

The Adapter retains each terminal `UnaryResponse`, terminal `OperationEvent`,
rejected `SourceOpenResult`, or `SourceClose` until the Host returns an exact
`TerminalAcknowledgement`. Its `terminal_digest` is
`sha256(fully-qualified-message-name || NUL || deterministic-protobuf-bytes)`,
formatted as `sha256:` plus lowercase hexadecimal. The Host acknowledges unary
and operation terminals only after the durable invocation commit. A normal
source close is acknowledged only after every preceding `SourceData` frame has
been accepted by the domain consumer; rejected opens and canceled-source closes
can be acknowledged immediately after their exact connection-local terminal
commit. Matching terminal replay receives the same acknowledgement; conflicting
bytes are a protocol violation.

`xgc.v1.Message` is domain-neutral. Robot and channel routing now lives only in
`xgc.robot.v1.RobotMessage`. `xgc.robot.v1.RobotAdapterSpec` is the typed robot
configuration payload for a generic `AdapterInstanceSpec`; it owns asset digest,
robot resources, profile identity, parameters, and channel grants without
leaking those fields into the Runtime Link. Semantic payload definitions and
their historical message IDs remain reusable and are not coupled to Adapter
instance scope.

See [docs/architecture.md](docs/architecture.md) for protocol invariants.

## Generate

Required tools:

- `protoc`, the Protobuf C++ development package, and `grpc_cpp_plugin`;
- Python `grpcio-tools`, `protobuf`, and `PyYAML`;
- pinned `protoc-gen-go` and `protoc-gen-go-grpc`.

```bash
python3 -m pip install -r requirements/generator.txt
tools/install-go-plugins.sh
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

Generated language bindings are ignored by Git. The Debian schema package also
does not install language bindings; each consumer generates bindings with its
pinned toolchain.

## Verify

```bash
buf lint
tools/smoke-test.sh
```

The smoke test:

- generates C++, Go, Python, gRPC, descriptors, and registries;
- verifies semantic payload registry round trips in C++, Go, and Python;
- verifies the robot-domain wrapper around a generic message;
- exercises Register, Control, and Work over an in-memory Go gRPC transport;
- verifies exact contract identity, operation deadline/idempotency, and
  independent byte/message stream credit with endpoint-schema-defined opaque
  stream items;
- verifies terminal acknowledgement identity and domain-separated digest shape;
- rejects domain-specific imports and descriptor names (robot, ROS, gateway,
  workflow, telemetry, UI, and related asset/run concepts) from the complete
  Adapter Runtime Link surface.

CI also runs `buf breaking` inside a protocol epoch. Product version `0.5.0-1`
starts a deliberate pre-1.0 breaking epoch for Runtime Link protocol v2 and its
Host-initiated, source-only stream contract.

## Debian schema development package

`xgc2-protobuf-dev` (`Architecture: all`) supports Ubuntu Focal, Jammy, and
Noble. It installs:

- `.proto` source files under `/usr/share/xgc2-protobuf/proto`;
- the complete descriptor set;
- source and generated JSON semantic message registries;
- CMake and pkg-config discovery metadata;
- protocol documentation.

It owns no native profile directory and installs no generated C++, Go, or Python
runtime bindings.

```bash
sudo apt update
sudo apt install xgc2-protobuf-dev
pkg-config --variable=proto_root xgc2-protobuf
```
