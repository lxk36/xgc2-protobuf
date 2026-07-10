#!/usr/bin/env python3

from xgc.registry.v1 import message_registry
from xgc.semantic.aerial.v1 import flight_pb2
from xgc.v1 import message_pb2


def main():
    payload = flight_pb2.FlightModeRequest(mode="OFFBOARD").SerializeToString()
    metadata = message_registry.METADATA[3111]
    envelope = message_pb2.Message(
        robot_id="uav1",
        channel_id="flight.set_mode",
        message_id=metadata["id"],
        schema_version=metadata["version"],
        schema_fingerprint=metadata["fingerprint"],
        encoding=message_pb2.PAYLOAD_ENCODING_PROTOBUF,
        payload=payload,
    )
    decoded = message_registry.new_message(envelope.message_id)
    decoded.ParseFromString(envelope.payload)
    assert decoded.mode == "OFFBOARD"
    assert message_registry.new_message(999999) is None
    print(
        "python roundtrip ok: {} {} {}".format(
            envelope.robot_id, envelope.channel_id, decoded.mode
        )
    )


if __name__ == "__main__":
    main()
