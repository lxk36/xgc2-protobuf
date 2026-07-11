# MAVLink command pass-through

XGC2 passes MAVLink commands at the command level, not at the serialized frame
level. The stable `xgc.v1.Message` envelope carries either:

- `xgc.mavlink.v1.CommandLongRequest` with a numeric `MAV_CMD` and parameters;
- `xgc.mavlink.v1.CommandAck` with the numeric `MAV_RESULT` and native ACK
  detail.

MAVLink packet version, source IDs, sequence numbers, CRC, signing, retries,
and link routing remain owned by MAVROS or another native Adapter. The
envelope's `robot_id` selects the Adapter-bound vehicle.

This follows the MAVLink command protocol: a command ID plus up to seven
parameters is sent in `COMMAND_LONG`, and the vehicle reports the result with
`COMMAND_ACK`. It also lets a new dialect add `MAV_CMD` values without forcing
an XGC2 protobuf change.

Official reference:
[MAVLink Command Protocol](https://mavlink.io/en/services/command.html).

## PX4 examples

Arm:

```text
command = 400  (MAV_CMD_COMPONENT_ARM_DISARM)
param1  = 1
param2  = 0
```

Disarm uses `param1 = 0`. Forced arm/disarm uses the MAVLink magic value
`param2 = 21196` and must be separately authorized; it must not be exposed by
an ordinary arm/disarm UI.

PX4 OFFBOARD mode:

```text
command = 176  (MAV_CMD_DO_SET_MODE)
param1  = 1    (MAV_MODE_FLAG_CUSTOM_MODE_ENABLED)
param2  = 6    (PX4_CUSTOM_MAIN_MODE_OFFBOARD)
param3  = 0
```

The numeric custom mode is PX4-specific. A future ArduPilot or other autopilot
profile can use different parameters without changing the protobuf.

## MAVROS mapping

The ROS1 PX4 profile maps one `mavlink.command_long` request/response channel
to:

```text
mavros/cmd/command  mavros_msgs/CommandLong
```

The installed MAVROS service returns `success` and the raw `COMMAND_ACK.result`
but does not expose ACK progress, `result_param2`, or target extension fields.
The Adapter therefore sets `CommandAck.progress = 255` and leaves unavailable
extension fields at zero. A direct MAVLink Adapter may populate all fields.

A ROS service transport failure produces an `OperationEvent` transport error
without a fabricated `CommandAck`.

## ACK is not observed state

`MAV_RESULT_ACCEPTED` means that the command was accepted, not necessarily that
the requested vehicle state has already been reached. Therefore:

- the ACK operation event is `OPERATION_PHASE_ACCEPTED`;
- actual arming and mode state continues to come from `FlightStatus`;
- a higher-level Go intent may wait for `armed` or `mode` telemetry before
  declaring its own workflow complete.

`MAV_RESULT_IN_PROGRESS` maps to `OPERATION_PHASE_STARTED`. Rejection, failure,
and cancellation map to their matching operation phases while preserving the
native numeric result in `CommandAck`.

## Authorization

Wire openness must not mean unrestricted command execution. The current PX4
profile permits only:

```text
176  MAV_CMD_DO_SET_MODE
400  MAV_CMD_COMPONENT_ARM_DISARM
```

Broadcast is disabled. Adding reboot, calibration, mission, camera, or other
commands is a profile and authorization change, not a protobuf change.

`COMMAND_INT` should be added as a separate protocol-native payload when XGC2
needs positional commands. It preserves coordinate frame and integer
latitude/longitude precision and should not be folded ambiguously into
`CommandLongRequest`.
