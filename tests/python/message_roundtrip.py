#!/usr/bin/env python3

from xgc.registry.v1 import message_registry
from xgc.semantic.aerial.v1 import control_pb2
from xgc.v1 import message_pb2


def main():
    payload = control_pb2.ModeRequest(mode="OFFBOARD").SerializeToString()
    metadata = message_registry.METADATA[3202]
    envelope = message_pb2.Message(
        robot_id="uav1",
        channel_id="operation.mode",
        message_id=metadata["id"],
        payload=payload,
    )
    decoded = message_registry.new_message(envelope.message_id)
    decoded.ParseFromString(envelope.payload)
    assert decoded.mode == "OFFBOARD"
    assert isinstance(message_registry.new_message(3201), control_pb2.ArmRequest)
    assert isinstance(
        message_registry.new_message(3203), control_pb2.AutopilotRebootRequest
    )
    assert message_registry.new_message(5001) is None
    assert message_registry.new_message(999999) is None
    print(
        "python roundtrip ok: {} {} mode={}".format(
            envelope.robot_id, envelope.channel_id, decoded.mode
        )
    )


if __name__ == "__main__":
    main()
