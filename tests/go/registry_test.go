package registryv1

import (
	"testing"

	"google.golang.org/protobuf/proto"
	aerialv1 "xgc2/protocols/xgc/semantic/aerial/v1"
	xgcv1 "xgc2/protocols/xgc/v1"
)

func TestFlightModeRoundTripThroughMessage(t *testing.T) {
	payload, err := proto.Marshal(&aerialv1.FlightModeRequest{Mode: "OFFBOARD"})
	if err != nil {
		t.Fatal(err)
	}
	metadata, ok := Lookup(3111)
	if !ok {
		t.Fatal("flight mode message is not registered")
	}
	envelope := &xgcv1.Message{
		RobotId:           "uav1",
		ChannelId:         "flight.set_mode",
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
	request, ok := decoded.(*aerialv1.FlightModeRequest)
	if !ok || request.GetMode() != "OFFBOARD" {
		t.Fatalf("unexpected decoded payload: %#v", decoded)
	}
	if _, ok := New(999999); ok {
		t.Fatal("unknown message ID should stay opaque")
	}
}
