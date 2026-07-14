package registryv1

import (
	"testing"

	"google.golang.org/protobuf/proto"
	aerialv1 "xgc2/protocols/xgc/semantic/aerial/v1"
	xgcv1 "xgc2/protocols/xgc/v1"
)

func TestAerialOperationRoundTripThroughMessage(t *testing.T) {
	payload, err := proto.Marshal(&aerialv1.ModeRequest{Mode: "OFFBOARD"})
	if err != nil {
		t.Fatal(err)
	}
	metadata, ok := Lookup(3202)
	if !ok {
		t.Fatal("mode request is not registered")
	}
	envelope := &xgcv1.Message{
		RobotId:           "uav1",
		ChannelId:         "operation.mode",
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
	request, ok := decoded.(*aerialv1.ModeRequest)
	if !ok || request.GetMode() != "OFFBOARD" {
		t.Fatalf("unexpected decoded payload: %#v", decoded)
	}
}

func TestAerialOperationMessagesAreRegistered(t *testing.T) {
	tests := []struct {
		id      uint32
		message proto.Message
	}{
		{id: 3201, message: &aerialv1.ArmRequest{Armed: true}},
		{id: 3202, message: &aerialv1.ModeRequest{}},
		{id: 3203, message: &aerialv1.AutopilotRebootRequest{}},
	}
	for _, test := range tests {
		created, ok := New(test.id)
		if !ok {
			t.Fatalf("message ID %d is not registered", test.id)
		}
		if created.ProtoReflect().Descriptor().FullName() != test.message.ProtoReflect().Descriptor().FullName() {
			t.Fatalf("message ID %d resolved to %s", test.id, created.ProtoReflect().Descriptor().FullName())
		}
	}
	if _, ok := New(5001); ok {
		t.Fatal("raw MAVLink request ID must remain unavailable")
	}
	if _, ok := New(999999); ok {
		t.Fatal("unknown message ID should stay opaque")
	}
}
