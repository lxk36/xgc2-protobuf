#!/usr/bin/env python3

from xgc.mavlink.v1 import command_pb2
from xgc.registry.v1 import message_registry
from xgc.v1 import message_pb2


def main():
    payload = command_pb2.CommandLongRequest(
        command=176,
        param1=1,
        param2=6,
    ).SerializeToString()
    metadata = message_registry.METADATA[5001]
    envelope = message_pb2.Message(
        robot_id="uav1",
        channel_id="mavlink.command_long",
        message_id=metadata["id"],
        schema_version=metadata["version"],
        schema_fingerprint=metadata["fingerprint"],
        encoding=message_pb2.PAYLOAD_ENCODING_PROTOBUF,
        payload=payload,
    )
    decoded = message_registry.new_message(envelope.message_id)
    decoded.ParseFromString(envelope.payload)
    assert decoded.command == 176
    assert decoded.param1 == 1
    assert decoded.param2 == 6
    assert isinstance(message_registry.new_message(5099), command_pb2.CommandAck)
    assert message_registry.new_message(999999) is None
    print(
        "python roundtrip ok: {} {} command={}".format(
            envelope.robot_id, envelope.channel_id, decoded.command
        )
    )


if __name__ == "__main__":
    main()
