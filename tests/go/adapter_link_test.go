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
	regv1 "xgc2/protocols/xgc/registry/v1"
	aerialv1 "xgc2/protocols/xgc/semantic/aerial/v1"
	xgcv1 "xgc2/protocols/xgc/v1"
)

type prototypeAdapterLinkServer struct {
	UnimplementedAdapterLinkServer
	telemetry chan *xgcv1.Message
	events    chan *OperationEvent
}

func (s *prototypeAdapterLinkServer) RegisterAdapter(
	_ context.Context,
	request *RegisterAdapterRequest,
) (*RegisterAdapterResponse, error) {
	if request.GetBootstrapToken() == "" || len(request.GetSupportedProfiles()) == 0 {
		return &RegisterAdapterResponse{Message: "bootstrap token and profiles are required"}, nil
	}
	return &RegisterAdapterResponse{
		Accepted:                true,
		CoreId:                  "core-test",
		SessionId:               "session-test",
		SelectedProtocolVersion: 1,
		RegistryFingerprint:     regv1.RegistryFingerprint,
		PlanRevision:            7,
		HeartbeatIntervalMs:     1000,
		MaxBatchBytes:           1024 * 1024,
	}, nil
}

func (s *prototypeAdapterLinkServer) Heartbeat(
	_ context.Context,
	request *HeartbeatRequest,
) (*HeartbeatResponse, error) {
	return &HeartbeatResponse{
		Accepted:            request.GetSessionId() == "session-test",
		CoreUnixNanos:       time.Now().UnixNano(),
		CurrentPlanRevision: 7,
		ReloadPlan:          request.GetAppliedPlanRevision() != 7,
	}, nil
}

func (s *prototypeAdapterLinkServer) GetAdapterPlan(
	_ context.Context,
	request *GetAdapterPlanRequest,
) (*AdapterPlan, error) {
	return &AdapterPlan{
		Accepted:    request.GetSessionId() == "session-test",
		Revision:    7,
		AssetDigest: "9ec2de7150c15f18085d42f8f18c22f6f0979a9ef84e873cfb8b11af858f85cb",
		Robots: []*RobotPlan{{
			RobotId:       "uav1",
			ProfileId:     "px4.multirotor.ros1.v1",
			ProfileDigest: "0000000000000000000000000000000000000000000000000000000000000000",
			Parameters:    map[string]string{"namespace": "/uav1"},
		}},
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
		Accepted:      batch.GetPlanRevision() == 7,
		BatchId:       batch.GetBatchId(),
		AcceptedCount: uint32(len(batch.GetMessages())),
	}, nil
}

func (s *prototypeAdapterLinkServer) StreamOperations(
	request *OperationStreamRequest,
	stream grpc.ServerStreamingServer[OperationRequest],
) error {
	if request.GetAppliedPlanRevision() != 7 {
		return nil
	}
	metadata, _ := regv1.Lookup(3202)
	payload, _ := proto.Marshal(&aerialv1.ModeRequest{Mode: "OFFBOARD"})
	return stream.Send(&OperationRequest{
		OperationId:       "op-mode-1",
		IssuedUnixNanos:   time.Now().UnixNano(),
		DeadlineUnixNanos: time.Now().Add(time.Second).UnixNano(),
		PlanRevision:      7,
		Message: &xgcv1.Message{
			RobotId:           "uav1",
			ChannelId:         "operation.mode",
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
		Accepted:      batch.GetPlanRevision() == 7,
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
	go func() { _ = grpcServer.Serve(listener) }()
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
		SoftwareVersion:           "0.2.0",
		SupportedProtocolVersions: []uint32{1},
		RegistryFingerprint:       regv1.RegistryFingerprint,
		BootstrapToken:            "single-use-bootstrap-token",
		SupportedProfiles: []*ProfileAdvertisement{{
			ProfileId:     "px4.multirotor.ros1.v1",
			ProfileDigest: "0000000000000000000000000000000000000000000000000000000000000000",
		}},
	})
	if err != nil || !registered.GetAccepted() || registered.GetSessionId() == "" {
		t.Fatalf("register failed: response=%v error=%v", registered, err)
	}

	plan, err := client.GetAdapterPlan(ctx, &GetAdapterPlanRequest{
		AdapterId: "ros1-adapter",
		SessionId: registered.GetSessionId(),
	})
	if err != nil || !plan.GetAccepted() || plan.GetRevision() != 7 || len(plan.GetRobots()) != 1 {
		t.Fatalf("plan failed: response=%v error=%v", plan, err)
	}
	if plan.GetRobots()[0].GetProfileId() != "px4.multirotor.ros1.v1" || plan.GetAssetDigest() == "" {
		t.Fatalf("unexpected asset-backed plan: %v", plan)
	}

	heartbeat, err := client.Heartbeat(ctx, &HeartbeatRequest{
		AdapterId:           "ros1-adapter",
		SessionId:           registered.GetSessionId(),
		AppliedPlanRevision: plan.GetRevision(),
	})
	if err != nil || !heartbeat.GetAccepted() || heartbeat.GetReloadPlan() {
		t.Fatalf("heartbeat failed: response=%v error=%v", heartbeat, err)
	}

	statusMetadata, _ := regv1.Lookup(3001)
	statusPayload, _ := proto.Marshal(&aerialv1.FlightStatus{
		Connected: true,
		Armed:     false,
		Mode:      "MANUAL",
	})
	ack, err := client.PushTelemetry(ctx, &TelemetryBatch{
		AdapterId:    "ros1-adapter",
		SessionId:    registered.GetSessionId(),
		BatchId:      1,
		PlanRevision: plan.GetRevision(),
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
		AdapterId:           "ros1-adapter",
		SessionId:           registered.GetSessionId(),
		RobotIds:            []string{"uav1"},
		AppliedPlanRevision: plan.GetRevision(),
	})
	if err != nil {
		t.Fatal(err)
	}
	operation, err := stream.Recv()
	if err != nil {
		t.Fatal(err)
	}
	if operation.GetPlanRevision() != plan.GetRevision() {
		t.Fatalf("operation omitted plan revision: %v", operation)
	}
	decoded, ok := regv1.New(operation.GetMessage().GetMessageId())
	if !ok || proto.Unmarshal(operation.GetMessage().GetPayload(), decoded) != nil {
		t.Fatalf("operation payload could not be decoded: %v", operation)
	}
	mode := decoded.(*aerialv1.ModeRequest)
	if mode.GetMode() != "OFFBOARD" {
		t.Fatalf("unexpected mode request: %v", mode)
	}

	eventAck, err := client.ReportOperationEvents(ctx, &OperationEventBatch{
		AdapterId:    "ros1-adapter",
		SessionId:    registered.GetSessionId(),
		BatchId:      2,
		PlanRevision: plan.GetRevision(),
		Events: []*OperationEvent{{
			OperationId: operation.GetOperationId(),
			Phase:       OperationPhase_OPERATION_PHASE_SUCCEEDED,
			Code:        ResultCode_RESULT_CODE_OK,
		}},
	})
	if err != nil || !eventAck.GetAccepted() || eventAck.GetAcceptedCount() != 1 {
		t.Fatalf("operation event report failed: response=%v error=%v", eventAck, err)
	}
	event := <-service.events
	if event.GetOperationId() != "op-mode-1" || event.GetPhase() != OperationPhase_OPERATION_PHASE_SUCCEEDED {
		t.Fatalf("unexpected operation event: %v", event)
	}
}
