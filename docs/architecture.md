# XGC2 protocol architecture

## Ownership and layers

The protocol product separates independently evolving layers:

1. `xgc.v1` owns domain-neutral schema references, payload encoding, time, and
   timestamped messages.
2. `xgc.adapter.v1` owns the capability-first Adapter Runtime Link.
3. `xgc.robot.v1` owns robot/channel routing without changing the generic
   message or Adapter Runtime Link.
4. `xgc.semantic` owns reusable semantic payload schemas and `registry/` owns
   their stable numeric IDs.

Native profiles, endpoint names, middleware types, conversion processors, and
native safety policy are owned by concrete Adapter products. They are not
installed by `xgc2-protobuf-dev`.

## Static capability contract

An Adapter registers immutable `CapabilityContract` values. Each endpoint
declares:

- unary, operation, or stream-source interaction;
- input, output, and event schemas;
- read-only, idempotent, or non-idempotent side effects;
- idempotency and cancellation support;
- deadline policy and size/concurrency/stream limits.

The trusted Adapter definition is the maximum grant. Registration is runtime
proof, not authority: the Host validates definition, build, manifest, SDK, and
contract digests before accepting the session.

## Instance and session fencing

The Host sends a complete `AdapterInstanceSpec` containing an immutable scope,
generic configuration, enabled endpoint set, secret references, revision, and
digest. Applying a spec is atomic. Until the exact revision/digest is reported
as applied, its capabilities are not ready. Every `CapabilityStatus` names the
exact contract by capability ID, contract version, and contract digest, so two
installed versions can never share ambiguous readiness.

Four counters identify independent failure and restart boundaries:

- `process_generation`: Process Supervisor generation;
- `session_generation`: Host activation generation for that instance;
- `runtime_epoch`: Host Runtime restart epoch;
- `connection_epoch`: reconnect generation for one paired Control+Work
  attachment.

Every stream frame also carries a monotonically increasing `frame_sequence`.
The Host rejects stale epochs, generations, sessions, or sequences. A process
restart, Host restart, session replacement, and stream reconnect therefore
cannot be mistaken for one another.

## Runtime flow

```text
prepared instance + one-time bootstrap credential
  -> Register(instance/process/build/manifest/SDK/capability proofs)
  -> fenced session
  -> Control stream
       Host -> complete revisioned instance spec, ping, drain, stop
       Adapter -> heartbeat, apply result, capability status
  -> Work stream
       Host -> unary/operation work, source open, source credit, cancellation
       Adapter -> unary result, operation event, source open result/data/close
```

Control and Work are established and fenced as a pair. They carry the same
`connection_epoch`; closing either stream replaces both, so an old half-pair
can never continue sending frames. Because a Work bidi stream can otherwise be
silent until the Host dispatches, its first Adapter-to-Host frame is a mandatory
`WorkAttach` carrying the complete session header and currently applied spec
revision/digest.

## Work semantics

### Unary

A unary request carries the immutable capability contract ID/version/digest,
endpoint identity, applied spec revision, subject reference, typed input,
deadline, idempotency key, and request digest. The same contract tuple is used
by operation and stream opens. The response contains exactly one typed output
or structured error.

### Operation

An operation uses the same work context and reports accepted, started,
progress, and one terminal phase. Side-effecting delivery can become
`ERROR_CLASS_UNCERTAIN` after dispatch. Deadline, rejected,
resource-exhausted, cancellation, transient, and permanent failures remain
machine distinguishable. Durable operation truth belongs to the Host; an
Adapter may retain only a bounded replay cache.

Unary and terminal operation frames remain Adapter-owned until the Host has
committed the exact terminal result to its durable invocation ledger and sent a
`TerminalAcknowledgement`. The acknowledgement identifies exactly one
`work_id`; its digest is `sha256:` plus lowercase hexadecimal SHA-256 over the
fully-qualified protobuf message name, one NUL byte, and the deterministic
protobuf bytes of the terminal submessage. Domain separation is mandatory:
different protobuf message types can otherwise serialize to identical bytes.
Matching replay is re-acknowledged, while a different digest for the same
terminal identity fails the paired connection.

Implementations must use their protobuf runtime's deterministic encoder. Go
uses `proto.MarshalOptions{Deterministic: true}`. C++ must call
`CodedOutputStream::SetSerializationDeterministic(true)` before
`SerializeToCodedStream`; ordinary `SerializeToString` is not a deterministic
digest implementation. In both cases the descriptor `full_name` bytes and one
NUL byte are hashed before the encoded message bytes.

### Stream

Runtime Link protocol v2 supports only Host-initiated source streams. The Host
validates the session fence, exact spec revision, capability contract, enabled
source endpoint, concurrency limit, and schema, allocates the stream ID, then
sends it as `SourceOpenRequest.context.work_id` with the first Adapter-to-Host
credit. The Adapter echoes that exact identity as `stream_id` in
`SourceOpenResult`, `SourceData`, and `SourceClose`. A successful result explicitly sets
`accepted=true`, even when it has no `initial_payload`; a rejected result carries
an `AdapterError`. The Host rejects accepted-plus-error, rejected-without-error,
and initial-payload-on-rejection combinations.

Only the Adapter sends `SourceData` and `SourceClose`. Only the Host sends
`SourceCredit` and `WorkCancellation`. Message and byte credit are independent
and both must remain positive before the Adapter transmits data.
`SourceData.items` are opaque bytes interpreted by the endpoint's output schema;
the base Link does not impose a domain routing envelope on stream items.

A rejected `SourceOpenResult` and every `SourceClose` use the same exact
terminal acknowledgement protocol with `stream_id`. Source terminal truth is
connection-local rather than a durable invocation. The Host first commits the
exact tombstone and publishes the terminal event to the subscriber. For a
normal close, it then waits until domain acknowledgements cover
`final_sequence`; the final `SourceCredit` is queued before the terminal
acknowledgement. A rejected open or Host-canceled source has no remaining
creditable data and can be acknowledged immediately. Once the exact terminal
acknowledgement enters the bounded Work queue, the unacknowledged tombstone slot
is released; only already-acknowledged replay records may be evicted.

There are no sink, duplex, Adapter-initiated-open, or direction-neutral stream
frames. Supporting any of those interactions requires a future protocol epoch
with explicit ownership and flow-control semantics.

## Domain boundary

`xgc.v1.Message` contains only sequence, source/observation time, and a typed
payload. `xgc.robot.v1.RobotMessage` adds `robot_id` and `channel_id` for robot
domain consumers. Other domains can define their own wrappers without changing
the common message or Runtime Link.

`ScopeReference` preserves the domain's canonical key and the complete
attribute map used to derive it. `SecretReference` preserves name, opaque
reference, and immutable version. These fields pass through the Runtime Link
without domain interpretation, avoiding lossy spec persistence.

## Process bootstrap

The Process Supervisor writes a binary `AdapterProcessBootstrap` file with mode
0600 and starts the process with `--adapter-bootstrap-file PATH`. It contains the
Runtime target, trusted `RegisterRequest`, and first complete
`AdapterInstanceSpec`. The SDK validates identity, process generation, contract
grants, scope attributes, and pinned secret versions before connecting; native
applications register handlers but do not synthesize identity proofs.

`xgc.robot.v1.RobotAdapterSpec` is a typed domain configuration carried in
`AdapterInstanceSpec.configuration`. It contains the immutable asset digest,
robot resources, profile IDs/digests, parameters, and channel grants. The Host
and Robot Adapter can therefore apply complete immutable domain configuration
without an untyped JSON map, while the base Runtime Link remains domain-free.

Large files and artifacts remain outside this protocol. A capability may define
a bounded stream of typed messages, but the Runtime Link is not a generic file
transfer or unbounded telemetry transport.
