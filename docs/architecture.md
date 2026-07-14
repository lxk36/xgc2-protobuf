# XGC2 protocol architecture

## Ownership and layers

The protocol product is the only source of truth for the Core-to-Adapter wire
contract. It separates three independently evolving layers:

1. `xgc.v1.Message` carries routing, time, message identity, and an opaque
   serialized payload. It is stable and direction-neutral.
2. `xgc.adapter.v1.AdapterLink` defines registration, session heartbeat, an
   asset-backed robot plan, telemetry batches, operations, and operation events.
3. Semantic payload messages define reusable XGC2 meaning. ROS topics,
   services, message types, conversions, and native command constants exist
   only in validated Adapter profiles and their processors.

The Go Core owns runtime binding and robot truth. A single-use bootstrap token
associates an Adapter registration with a server-side execution target,
experiment, orchestration run, swarm asset, and asset revision. None of those
identifiers are accepted from the Adapter as a replacement source of truth.

An Adapter registers supported profiles and their exact raw-byte digests. It
does not advertise robot inventory. Core returns the robots selected from the
bound swarm asset in `AdapterPlan`; every robot entry names a supported profile,
its digest, and schema-defined parameters. The plan revision and asset digest
make the applied configuration auditable.

## Runtime flow

```text
Adapter process bootstrap token
  -> RegisterAdapter(supported profiles and digests)
  -> authenticated session
  -> GetAdapterPlan
  -> asset-backed RobotPlan entries
  -> Heartbeat(applied plan revision)

ROS topics
  -> typed Adapter callbacks
  -> cache / synchronization / validation / rate reduction
  -> semantic protobuf payloads
  -> xgc.v1.Message
  -> TelemetryBatch(session and plan revision)
  -> Go Core

Go operation
  -> validated semantic xgc.v1.Message
  -> OperationRequest(plan revision)
  -> Adapter profile processor
  -> ROS publisher or service client
  -> OperationEvent
  -> Go Core
```

One telemetry RPC may contain several independently timestamped semantic
messages. This avoids both one-proto-per-ROS-message mirroring and a single
ever-growing robot-state aggregate.

## Plan and operation invariants

- A session is valid only for its server-side runtime binding.
- Registration selects one exact registry fingerprint for the whole session;
  per-message schema, fingerprint, and encoding fields are not carried.
- A plan is accepted atomically; partial robot configuration is invalid.
- `profile_digest` must equal the lowercase SHA-256 of the complete profile YAML
  raw bytes installed at the Adapter.
- Telemetry and operation-event batches carry the applied plan revision.
- Every operation carries the plan revision from which its robot and channel
  authorization were derived.
- An Adapter rejects operations for an unknown robot, disabled channel, stale
  plan revision, or unsupported message ID.
- The Core never sends a ROS topic, service, native type, or arbitrary native
  command through an operation.

## Unknown message behavior

- Unknown telemetry may be transported, recorded, and forwarded as opaque
  bytes only after session, robot, channel, size, and rate validation.
- Unknown or unadvertised control messages are rejected.
- A message ID is never reused; removed IDs remain reserved.
- A breaking payload change receives a new schema version or message ID.
- Native ROS names and types are not accepted from individual operations;
  operations reference a channel advertised by the selected profile.

## Data-channel boundary

The contract carries structured control, telemetry, health, and diagnostics.
Large images, point clouds, maps, bags, and files use dedicated streaming or
artifact transports and are intentionally outside `AdapterLink`.
