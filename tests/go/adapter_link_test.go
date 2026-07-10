package adapterv1

import (
	"context"
	"net"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/proto"
	registryv1 "xgc2/protocols/xgc/registry/v1"
	aerialv1 "xgc2/protocols/xgc/semantic/aerial/v1"
	xgcv1 "xgc2/protocols/xgc/v1"
)

type prototypeAdapterLinkServer struct {
	UnimplementedAdapterLinkServer
	telemetry chan *xgcv1.Message
	events    chan *OperationEvent
}

func (s *prototypeAdapterLinkServer) RegisterAdapter(
	context.Context,
	*RegisterAdapterRequest,
) (*RegisterAdapterResponse, error) {
	return &RegisterAdapterResponse{
		Accepted:                true,
		CoreId:                  "core-test",
		SessionId:               "session-test",
		SelectedProtocolVersion: 1,
		RegistryFingerprint:     registryv1.RegistryFingerprint,
		PlanRevision:            1,
	}, nil
}

func (s *prototypeAdapterLinkServer) PushTelemetry(
	_ context.Context,
	batch *TelemetryBatch,
) (*BatchAck, error) {
	for _, message := range batch.GetMessages() {
		s.telemetry <- message
	}
	return &BatchAck{
		Accepted:      true,
		BatchId:       batch.GetBatchId(),
		AcceptedCount: uint32(len(batch.GetMessages())),
	}, nil
}

func (s *prototypeAdapterLinkServer) StreamOperations(
	_ *OperationStreamRequest,
	stream grpc.ServerStreamingServer[OperationRequest],
) error {
	metadata, _ := registryv1.Lookup(3111)
	payload, _ := proto.Marshal(&aerialv1.FlightModeRequest{Mode: "OFFBOARD"})
	return stream.Send(&OperationRequest{
		OperationId:       "op-mode-1",
		IssuedUnixNanos:   time.Now().UnixNano(),
		DeadlineUnixNanos: time.Now().Add(time.Second).UnixNano(),
		Message: &xgcv1.Message{
			RobotId:           "uav1",
			ChannelId:         "flight.set_mode",
			MessageId:         metadata.ID,
			SchemaVersion:     metadata.Version,
			SchemaFingerprint: metadata.Fingerprint,
			Encoding:          xgcv1.PayloadEncoding_PAYLOAD_ENCODING_PROTOBUF,
			Payload:           payload,
		},
	})
}

func (s *prototypeAdapterLinkServer) ReportOperationEvents(
	_ context.Context,
	batch *OperationEventBatch,
) (*BatchAck, error) {
	for _, event := range batch.GetEvents() {
		s.events <- event
	}
	return &BatchAck{
		Accepted:      true,
		BatchId:       batch.GetBatchId(),
		AcceptedCount: uint32(len(batch.GetEvents())),
	}, nil
}

func TestAdapterLinkEndToEnd(t *testing.T) {
	listener := bufconn.Listen(1024 * 1024)
	grpcServer := grpc.NewServer()
	service := &prototypeAdapterLinkServer{
		telemetry: make(chan *xgcv1.Message, 1),
		events:    make(chan *OperationEvent, 1),
	}
	RegisterAdapterLinkServer(grpcServer, service)
	go func() {
		_ = grpcServer.Serve(listener)
	}()
	t.Cleanup(grpcServer.Stop)

	connection, err := grpc.NewClient(
		"passthrough:///xgc2-protocol-test",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return listener.Dial()
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = connection.Close() })
	client := NewAdapterLinkClient(connection)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	registered, err := client.RegisterAdapter(ctx, &RegisterAdapterRequest{
		AdapterId:                 "ros1-adapter",
		NativeProtocol:            "ros1",
		SupportedProtocolVersions: []uint32{1},
		RegistryFingerprint:       registryv1.RegistryFingerprint,
	})
	if err != nil || !registered.GetAccepted() || registered.GetSessionId() == "" {
		t.Fatalf("register failed: response=%v error=%v", registered, err)
	}

	statusMetadata, _ := registryv1.Lookup(3001)
	statusPayload, _ := proto.Marshal(&aerialv1.FlightStatus{
		Connected: true,
		Armed:     false,
		Mode:      "MANUAL",
	})
	ack, err := client.PushTelemetry(ctx, &TelemetryBatch{
		AdapterId: "ros1-adapter",
		SessionId: registered.GetSessionId(),
		BatchId:   1,
		Messages: []*xgcv1.Message{{
			RobotId:           "uav1",
			ChannelId:         "state.flight",
			MessageId:         statusMetadata.ID,
			SchemaVersion:     statusMetadata.Version,
			SchemaFingerprint: statusMetadata.Fingerprint,
			Encoding:          xgcv1.PayloadEncoding_PAYLOAD_ENCODING_PROTOBUF,
			Payload:           statusPayload,
		}},
	})
	if err != nil || !ack.GetAccepted() || ack.GetAcceptedCount() != 1 {
		t.Fatalf("telemetry push failed: response=%v error=%v", ack, err)
	}
	if uploaded := <-service.telemetry; uploaded.GetMessageId() != 3001 {
		t.Fatalf("unexpected uploaded telemetry: %v", uploaded)
	}

	stream, err := client.StreamOperations(ctx, &OperationStreamRequest{
		AdapterId: "ros1-adapter",
		SessionId: registered.GetSessionId(),
		RobotIds:  []string{"uav1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	operation, err := stream.Recv()
	if err != nil {
		t.Fatal(err)
	}
	decoded, ok := registryv1.New(operation.GetMessage().GetMessageId())
	if !ok || proto.Unmarshal(operation.GetMessage().GetPayload(), decoded) != nil {
		t.Fatalf("operation payload could not be decoded: %v", operation)
	}
	mode := decoded.(*aerialv1.FlightModeRequest)
	if mode.GetMode() != "OFFBOARD" {
		t.Fatalf("unexpected mode request: %v", mode)
	}

	eventAck, err := client.ReportOperationEvents(ctx, &OperationEventBatch{
		AdapterId: "ros1-adapter",
		SessionId: registered.GetSessionId(),
		BatchId:   2,
		Events: []*OperationEvent{{
			OperationId: operation.GetOperationId(),
			Phase:       OperationPhase_OPERATION_PHASE_SUCCEEDED,
			Code:        ResultCode_RESULT_CODE_OK,
		}},
	})
	if err != nil || eventAck.GetAcceptedCount() != 1 {
		t.Fatalf("operation event report failed: response=%v error=%v", eventAck, err)
	}
	if event := <-service.events; event.GetOperationId() != "op-mode-1" {
		t.Fatalf("unexpected operation event: %v", event)
	}
}
