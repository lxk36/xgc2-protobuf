# XGC2 protocol prototype architecture

## Layers

The prototype deliberately separates three independently evolving layers:

1. `xgc.v1.Message` carries routing, time, message identity, and an
   opaque serialized payload. It is stable and direction-neutral.
2. `xgc.adapter.v1.AdapterLink` defines the Go Core to Adapter lifecycle and
   transport flows: registration, plans, telemetry batches, operations, and
   operation events.
3. Payload protobuf messages either define reusable XGC2 meaning or an
   explicitly named protocol-native dialect boundary such as MAVLink command
   pass-through. ROS-specific topics, services, and conversions live in
   Adapter profiles and processors.

The message payload uses protobuf binary encoding in this prototype. A new
payload message extends the registry and generated language packages without
changing `xgc.v1.Message` or the gRPC service.

## Runtime flow

```text
ROS topics
  -> typed C++ callbacks
  -> adapter cache / synchronization / validation / rate reduction
  -> semantic protobuf payloads
  -> xgc.v1.Message
  -> TelemetryBatch
  -> Go Core

Go operation
  -> xgc.v1.Message
  -> OperationRequest stream
  -> adapter payload decoder
  -> ROS publisher or service client
  -> OperationEvent
  -> Go Core
```

For MAVLink commands, Go sends `xgc.mavlink.v1.CommandLongRequest`; the ROS1
profile maps it to `mavros_msgs/CommandLong`, and the Adapter returns the raw
MAVLink result in `xgc.mavlink.v1.CommandAck`. MAVLink framing remains below
the Adapter boundary.

One telemetry RPC may contain several independently timestamped semantic
messages. This avoids both one-proto-per-ROS-message mirroring and a single
ever-growing robot state aggregate.

## Unknown message behavior

- Unknown telemetry may be transported, recorded, and forwarded as opaque
  bytes.
- Unknown or unadvertised control messages must be rejected.
- Protocol-native pass-through commands must also satisfy the selected
  profile's command whitelist.
- A message ID is never reused.
- A breaking payload change gets a new schema version or message ID.
- Native ROS names and types are not accepted from individual operations;
  operations reference an advertised channel.

## Prototype limitations

- Adapter profile YAML is illustrative and does not yet have a formal schema.
- Registry fingerprints are generated from protobuf descriptors, but package
  compatibility policy is not frozen.
- Plan authorization and endpoint override policy still need a threat-model
  review.
- Large image, point-cloud, map, and file payloads are intentionally outside
  this protocol.
