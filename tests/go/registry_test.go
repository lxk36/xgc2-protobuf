package registryv1

import (
	"testing"

	"google.golang.org/protobuf/proto"
	mavlinkv1 "xgc2/protocols/xgc/mavlink/v1"
	xgcv1 "xgc2/protocols/xgc/v1"
)

func TestMavlinkCommandRoundTripThroughMessage(t *testing.T) {
	payload, err := proto.Marshal(&mavlinkv1.CommandLongRequest{
		Command: 176,
		Param1:  1,
		Param2:  6,
	})
	if err != nil {
		t.Fatal(err)
	}
	metadata, ok := Lookup(5001)
	if !ok {
		t.Fatal("MAVLink command message is not registered")
	}
	envelope := &xgcv1.Message{
		RobotId:           "uav1",
		ChannelId:         "mavlink.command_long",
		MessageId:         metadata.ID,
		SchemaVersion:     metadata.Version,
		SchemaFingerprint: metadata.Fingerprint,
		Encoding:          xgcv1.PayloadEncoding_PAYLOAD_ENCODING_PROTOBUF,
		Payload:           payload,
	}
	decoded, ok := New(envelope.GetMessageId())
	if !ok {
		t.Fatal("registered message could not be constructed")
	}
	if err := proto.Unmarshal(envelope.GetPayload(), decoded); err != nil {
		t.Fatal(err)
	}
	request, ok := decoded.(*mavlinkv1.CommandLongRequest)
	if !ok || request.GetCommand() != 176 || request.GetParam1() != 1 || request.GetParam2() != 6 {
		t.Fatalf("unexpected decoded payload: %#v", decoded)
	}
	ack, ok := New(5099)
	if !ok {
		t.Fatal("MAVLink command ACK is not registered")
	}
	if _, ok := ack.(*mavlinkv1.CommandAck); !ok {
		t.Fatalf("unexpected ACK payload type: %#v", ack)
	}
	if _, ok := New(999999); ok {
		t.Fatal("unknown message ID should stay opaque")
	}
}
