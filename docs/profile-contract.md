# Adapter profile contract

Adapter profiles are validated against
`profiles/schema/adapter-profile-v1.schema.json`. A profile owns all native ROS
topic, service, message-type, conversion, timeout, and safety policy details.
Core works only with `profile_id`, semantic channel IDs, registered message IDs,
and schema-defined plan parameters.

## Profile digest

The profile digest algorithm is `sha256-raw-bytes`:

1. Read the complete profile YAML file as raw bytes.
2. Do not parse, re-serialize, trim, normalize Unicode, rewrite line endings, or
   omit comments.
3. Compute SHA-256 over those bytes.
4. Encode the result as exactly 64 lowercase hexadecimal characters with no
   prefix.

Consequently, whitespace, comments, final newlines, and line-ending changes all
produce a different digest. This is intentional: the digest identifies the
exact installed profile artifact rather than an interpretation of it.

`tools/validate_profiles.py` writes `generated/profile-registry.json`. The
schema-only Debian package installs the equivalent registry at
`/usr/share/xgc2-protobuf/profiles/registry.json`. Each entry contains the
profile ID, profile version, native protocol, robot kind, channel IDs, source
path, digest algorithm, and raw-byte digest. The registry also contains:

- `profile_schema_digest`: raw-byte digest of the JSON Schema;
- `catalog_digest`: SHA-256 over sorted `profile_id`, NUL, profile digest, and
  newline tuples.

## Namespace and endpoints

Every profile declares a required `ros_namespace` parameter through
`namespace_parameter`. Endpoint names default to `robot_namespace` scope and
are resolved beneath that namespace by the Adapter. A profile may declare a
fixed `global` endpoint and substitute a required, schema-constrained parameter
as a complete path segment, for example
`vrpn_client_node/{mocap_rigid_body}/pose`. Absolute endpoint values remain
unavailable to Core, so a plan cannot inject an arbitrary ROS topic.

## Channel kinds

- `stream_out`: Adapter produces a semantic telemetry or diagnostic message
  from native inputs or from observed channel health.
- `stream_in`: Adapter consumes a semantic high-rate control message and
  publishes to a native endpoint.
- `request_response`: Adapter maps a semantic request to a native service and
  returns a semantic response payload.
- `operation`: Adapter maps a semantic request to a native service and reports
  lifecycle and native result information through `OperationEvent`.

The catalog contains:

- `px4.multirotor.ros1.v1`;
- `px4.multirotor.ros1.v2`;
- `scout-mini.ros1.v1`.

PX4 profile v2 adds mocap-to-MAVROS vision relay, local and attitude setpoint
observation, FCU timesync diagnostics, and aggregated stream health. The relay
preserves the VRPN pose and timestamp exactly, applies no coordinate transform,
publishes each source sample at most once, and caps output at 50 Hz.

PX4 arm, mode, and normal autopilot restart use semantic message IDs 3201, 3202,
and 3203. Raw MAVLink commands are not part of the public protocol. Scout Mini
currently exposes telemetry and channel diagnostics only; high-rate control and
operations are intentionally not enabled in this profile revision.
