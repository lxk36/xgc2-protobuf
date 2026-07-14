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

Every v1 profile declares a required `ros_namespace` parameter through
`namespace_parameter`. Endpoint names in the profile are relative and are
resolved beneath that namespace by the Adapter. Absolute endpoints are rejected
by schema validation so Core cannot inject a native ROS endpoint through plan
parameters.

## Channel kinds

- `stream_out`: Adapter produces a semantic telemetry or diagnostic message
  from native inputs or from observed channel health.
- `stream_in`: Adapter consumes a semantic high-rate control message and
  publishes to a native endpoint.
- `request_response`: Adapter maps a semantic request to a native service and
  returns a semantic response payload.
- `operation`: Adapter maps a semantic request to a native service and reports
  lifecycle and native result information through `OperationEvent`.

The initial hard-cut catalog contains only:

- `px4.multirotor.ros1.v1`;
- `scout-mini.ros1.v1`.

PX4 arm, mode, and normal autopilot restart use semantic message IDs 3201, 3202,
and 3203. Raw MAVLink commands are not part of the public protocol. Scout Mini
currently exposes telemetry and channel diagnostics only; high-rate control and
operations are intentionally not enabled in this profile revision.
