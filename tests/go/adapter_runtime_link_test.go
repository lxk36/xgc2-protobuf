package adapterv1

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	xgcv1 "xgc2/protocols/xgc/v1"
)

func TestTerminalDigestDomainSeparatesMessageTypes(t *testing.T) {
	open := &SourceOpenResult{StreamId: "stream-1", Accepted: true}
	closeFrame := &SourceClose{StreamId: "stream-1", FinalSequence: 1}
	marshal := proto.MarshalOptions{Deterministic: true}
	openBytes, err := marshal.Marshal(open)
	if err != nil {
		t.Fatal(err)
	}
	closeBytes, err := marshal.Marshal(closeFrame)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(openBytes, closeBytes) {
		t.Fatalf("digest collision fixture changed: open=%x close=%x", openBytes, closeBytes)
	}
	if testTerminalDigest(open) == testTerminalDigest(closeFrame) {
		t.Fatal("terminal digest did not domain-separate identical cross-type wire bytes")
	}
}

const (
	testInstanceID        = "adapter-instance-test"
	testSessionID         = "adapter-session-test"
	testProcessGeneration = uint64(7)
	testSessionGeneration = uint64(3)
	testRuntimeEpoch      = uint64(11)
	testCapabilityID      = "test.native"
	testContractVersion   = uint32(1)
	testContractDigest    = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
)

type prototypeAdapterRuntimeLinkServer struct {
	UnimplementedAdapterRuntimeLinkServiceServer
	registration chan *RegisterRequest
	response     chan *UnaryResponse
	sourceOpen   chan *SourceOpenResult
	sourceData   chan *SourceData
	sourceClose  chan *SourceClose
}

func testHeader(sequence uint64) *SessionHeader {
	return &SessionHeader{
		InstanceId: testInstanceID, SessionId: testSessionID,
		ProcessGeneration: testProcessGeneration, SessionGeneration: testSessionGeneration,
		RuntimeEpoch: testRuntimeEpoch, ConnectionEpoch: 1, FrameSequence: sequence,
	}
}

func testPayload(value string) *xgcv1.Payload {
	return &xgcv1.Payload{
		Schema: &xgcv1.SchemaReference{
			MessageId: 1001, TypeName: "xgc.test.v1.Value", SchemaVersion: 1,
			SchemaFingerprint: 0x1020304050607080,
		},
		Encoding: xgcv1.PayloadEncoding_PAYLOAD_ENCODING_PROTOBUF,
		Value:    []byte(value),
	}
}

func testTerminalDigest(message proto.Message) string {
	encoded, err := (proto.MarshalOptions{Deterministic: true}).Marshal(message)
	if err != nil {
		panic(err)
	}
	hash := sha256.New()
	_, _ = hash.Write([]byte(message.ProtoReflect().Descriptor().FullName()))
	_, _ = hash.Write([]byte{0})
	_, _ = hash.Write(encoded)
	return "sha256:" + hex.EncodeToString(hash.Sum(nil))
}

func testCapability() *CapabilityContract {
	return &CapabilityContract{
		CapabilityId: testCapabilityID, ContractVersion: testContractVersion, ContractDigest: testContractDigest,
		Endpoints: []*CapabilityEndpointContract{
			{
				EndpointId: "query", InteractionMode: InteractionMode_INTERACTION_MODE_UNARY,
				InputSchema: testPayload("").GetSchema(), OutputSchema: testPayload("").GetSchema(),
				SideEffectClass:  SideEffectClass_SIDE_EFFECT_CLASS_READ_ONLY,
				IdempotencyMode:  IdempotencyMode_IDEMPOTENCY_MODE_OPTIONAL,
				DeadlineRequired: true, MaximumTimeoutMs: 5000,
			},
			{
				EndpointId: "command", InteractionMode: InteractionMode_INTERACTION_MODE_OPERATION,
				InputSchema: testPayload("").GetSchema(), EventSchema: testPayload("").GetSchema(),
				SideEffectClass:       SideEffectClass_SIDE_EFFECT_CLASS_NON_IDEMPOTENT,
				IdempotencyMode:       IdempotencyMode_IDEMPOTENCY_MODE_REQUIRED,
				CancellationSupported: true, DeadlineRequired: true, MaximumTimeoutMs: 30000,
			},
			{
				EndpointId: "events", InteractionMode: InteractionMode_INTERACTION_MODE_STREAM_SOURCE,
				OutputSchema: testPayload("").GetSchema(),
				Limits:       &EndpointLimits{MaximumStreams: 1, MaximumStreamChunkBytes: 4096, MaximumStreamChunkMessages: 8},
			},
		},
	}
}

func (server *prototypeAdapterRuntimeLinkServer) Register(
	_ context.Context, request *RegisterRequest,
) (*RegisterResponse, error) {
	server.registration <- proto.Clone(request).(*RegisterRequest)
	return &RegisterResponse{
		Accepted: true, HostId: "host-test", SessionId: testSessionID,
		ProcessGeneration: testProcessGeneration, SessionGeneration: testSessionGeneration,
		RuntimeEpoch: testRuntimeEpoch, RuntimeLinkProtocolVersion: 2, CurrentSpecRevision: 4,
		HeartbeatIntervalMs: 1000, MaximumControlFrameBytes: 64 * 1024,
		MaximumWorkFrameBytes: 1024 * 1024,
	}, nil
}

func (server *prototypeAdapterRuntimeLinkServer) Control(
	stream grpc.BidiStreamingServer[ControlRequest, ControlResponse],
) error {
	request, err := stream.Recv()
	if err != nil {
		return err
	}
	heartbeat := request.GetHeartbeat()
	if heartbeat == nil || len(heartbeat.GetCapabilities()) != 1 {
		return nil
	}
	status := heartbeat.GetCapabilities()[0]
	if request.GetHeader().GetRuntimeEpoch() != testRuntimeEpoch ||
		status.GetContractVersion() != testContractVersion || status.GetContractDigest() != testContractDigest {
		return nil
	}
	return stream.Send(&ControlResponse{
		Header: testHeader(1),
		Frame: &ControlResponse_InstanceSpec{InstanceSpec: &AdapterInstanceSpec{
			InstanceId: testInstanceID, ProcessGeneration: testProcessGeneration,
			Revision: 4, SpecDigest: strings.Repeat("b", 64),
			Scope:         &ScopeReference{Kind: "target-context", Key: "context-test"},
			Configuration: testPayload("configuration"),
			Capabilities: []*EnabledCapability{{
				CapabilityId: testCapabilityID, ContractVersion: testContractVersion,
				ContractDigest:     testContractDigest,
				EnabledEndpointIds: []string{"query", "command", "events"},
			}},
		}},
	})
}

func (server *prototypeAdapterRuntimeLinkServer) Work(
	stream grpc.BidiStreamingServer[WorkRequest, WorkResponse],
) error {
	attach, err := stream.Recv()
	if err != nil {
		return err
	}
	if attach.GetAttach() == nil || attach.GetHeader().GetConnectionEpoch() != 1 ||
		attach.GetHeader().GetRuntimeEpoch() != testRuntimeEpoch {
		return nil
	}
	if err := stream.Send(&WorkResponse{
		Header: testHeader(1),
		Frame: &WorkResponse_UnaryRequest{UnaryRequest: &UnaryRequest{
			Context: &WorkContext{
				WorkId: "work-1", CapabilityId: testCapabilityID, EndpointId: "query",
				ContractVersion: testContractVersion, ContractDigest: testContractDigest,
				SpecRevision: 4, Deadline: &Deadline{DeadlineUnixNanos: time.Now().Add(time.Second).UnixNano(), TtlMs: 1000},
				IdempotencyKey: "query-1", RequestDigest: strings.Repeat("c", 64),
				Subject: &ScopeReference{Kind: "resource", Key: "resource-test"},
			},
			Input: testPayload("request"),
		}},
	}); err != nil {
		return err
	}
	request, err := stream.Recv()
	if err != nil {
		return err
	}
	server.response <- proto.Clone(request.GetUnaryResponse()).(*UnaryResponse)
	if err := stream.Send(&WorkResponse{
		Header: testHeader(2),
		Frame: &WorkResponse_TerminalAcknowledgement{TerminalAcknowledgement: &TerminalAcknowledgement{
			Identity:       &TerminalAcknowledgement_WorkId{WorkId: request.GetUnaryResponse().GetWorkId()},
			TerminalDigest: testTerminalDigest(request.GetUnaryResponse()),
		}},
	}); err != nil {
		return err
	}
	if err := stream.Send(&WorkResponse{
		Header: testHeader(3),
		Frame: &WorkResponse_SourceOpenRequest{SourceOpenRequest: &SourceOpenRequest{
			Context: &WorkContext{
				WorkId: "source-1", CapabilityId: testCapabilityID, EndpointId: "events",
				ContractVersion: testContractVersion, ContractDigest: testContractDigest,
				SpecRevision: 4, Deadline: &Deadline{DeadlineUnixNanos: time.Now().Add(time.Second).UnixNano(), TtlMs: 1000},
			},
			InitialCredit: &FlowControl{Messages: 2, Bytes: 1024},
		}},
	}); err != nil {
		return err
	}
	request, err = stream.Recv()
	if err != nil {
		return err
	}
	server.sourceOpen <- proto.Clone(request.GetSourceOpenResult()).(*SourceOpenResult)
	request, err = stream.Recv()
	if err != nil {
		return err
	}
	server.sourceData <- proto.Clone(request.GetSourceData()).(*SourceData)
	request, err = stream.Recv()
	if err != nil {
		return err
	}
	server.sourceClose <- proto.Clone(request.GetSourceClose()).(*SourceClose)
	return stream.Send(&WorkResponse{
		Header: testHeader(4),
		Frame: &WorkResponse_TerminalAcknowledgement{TerminalAcknowledgement: &TerminalAcknowledgement{
			Identity:       &TerminalAcknowledgement_StreamId{StreamId: request.GetSourceClose().GetStreamId()},
			TerminalDigest: testTerminalDigest(request.GetSourceClose()),
		}},
	})
}

func TestAdapterRuntimeLinkVerticalSlice(t *testing.T) {
	listener := bufconn.Listen(1024 * 1024)
	grpcServer := grpc.NewServer()
	server := &prototypeAdapterRuntimeLinkServer{
		registration: make(chan *RegisterRequest, 1), response: make(chan *UnaryResponse, 1),
		sourceOpen: make(chan *SourceOpenResult, 1), sourceData: make(chan *SourceData, 1),
		sourceClose: make(chan *SourceClose, 1),
	}
	RegisterAdapterRuntimeLinkServiceServer(grpcServer, server)
	go func() { _ = grpcServer.Serve(listener) }()
	t.Cleanup(grpcServer.Stop)

	connection, err := grpc.NewClient(
		"passthrough:///xgc2-adapter-runtime-test",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = connection.Close() })
	client := NewAdapterRuntimeLinkServiceClient(connection)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	registered, err := client.Register(ctx, &RegisterRequest{
		InstanceId: testInstanceID, ProcessGeneration: testProcessGeneration,
		DefinitionId: "test-adapter", DefinitionDigest: strings.Repeat("d", 64),
		BuildDigest: strings.Repeat("e", 64), ManifestDigest: strings.Repeat("f", 64),
		SoftwareVersion: "1.0.0", SdkVersion: "1.0.0", RuntimeLinkProtocolVersion: 2,
		BootstrapToken: "single-use-token", SupportedCapabilities: []*CapabilityContract{testCapability()},
	})
	if err != nil || !registered.GetAccepted() || registered.GetSessionGeneration() != testSessionGeneration {
		t.Fatalf("register failed: response=%v error=%v", registered, err)
	}
	advertised := <-server.registration
	if advertised.GetBuildDigest() == "" || len(advertised.GetSupportedCapabilities()) != 1 {
		t.Fatalf("registration omitted build proof or capability contract: %v", advertised)
	}

	control, err := client.Control(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := control.Send(&ControlRequest{
		Header: testHeader(1),
		Frame: &ControlRequest_Heartbeat{Heartbeat: &Heartbeat{
			ObservedUnixNanos: time.Now().UnixNano(), AppliedSpecRevision: 0,
			Capabilities: []*CapabilityStatus{{
				CapabilityId: testCapabilityID, ContractVersion: testContractVersion,
				ContractDigest: testContractDigest, State: CapabilityState_CAPABILITY_STATE_READY,
			}},
		}},
	}); err != nil {
		t.Fatal(err)
	}
	controlFrame, err := control.Recv()
	if err != nil || controlFrame.GetInstanceSpec().GetRevision() != 4 {
		t.Fatalf("control spec failed: frame=%v error=%v", controlFrame, err)
	}
	_ = control.CloseSend()

	work, err := client.Work(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := work.Send(&WorkRequest{
		Header: testHeader(2),
		Frame:  &WorkRequest_Attach{Attach: &WorkAttach{}},
	}); err != nil {
		t.Fatal(err)
	}
	workFrame, err := work.Recv()
	workContext := workFrame.GetUnaryRequest().GetContext()
	if err != nil || workContext.GetIdempotencyKey() == "" ||
		workContext.GetContractVersion() != testContractVersion || workContext.GetContractDigest() != testContractDigest {
		t.Fatalf("work request failed: frame=%v error=%v", workFrame, err)
	}
	if err := work.Send(&WorkRequest{
		Header: testHeader(1),
		Frame: &WorkRequest_UnaryResponse{UnaryResponse: &UnaryResponse{
			WorkId: "work-1", Outcome: &UnaryResponse_Output{Output: testPayload("response")},
		}},
	}); err != nil {
		t.Fatal(err)
	}
	response := <-server.response
	if string(response.GetOutput().GetValue()) != "response" {
		t.Fatalf("unexpected unary response: %v", response)
	}
	unaryAckFrame, err := work.Recv()
	unaryAck := unaryAckFrame.GetTerminalAcknowledgement()
	if err != nil || unaryAck.GetWorkId() != "work-1" || unaryAck.GetStreamId() != "" ||
		unaryAck.GetTerminalDigest() != testTerminalDigest(response) {
		t.Fatalf("unary terminal acknowledgement failed: frame=%v error=%v", unaryAckFrame, err)
	}
	sourceFrame, err := work.Recv()
	sourceRequest := sourceFrame.GetSourceOpenRequest()
	if err != nil || sourceRequest.GetContext().GetWorkId() != "source-1" ||
		sourceRequest.GetInitialCredit().GetMessages() != 2 {
		t.Fatalf("source open request failed: frame=%v error=%v", sourceFrame, err)
	}
	if err := work.Send(&WorkRequest{
		Header: testHeader(3),
		Frame: &WorkRequest_SourceOpenResult{SourceOpenResult: &SourceOpenResult{
			StreamId: "source-1", Accepted: true,
		}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := work.Send(&WorkRequest{
		Header: testHeader(4),
		Frame: &WorkRequest_SourceData{SourceData: &SourceData{
			StreamId: "source-1", Sequence: 1, Items: [][]byte{[]byte("typed-item")},
		}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := work.Send(&WorkRequest{
		Header: testHeader(5),
		Frame: &WorkRequest_SourceClose{SourceClose: &SourceClose{
			StreamId: "source-1", FinalSequence: 1,
		}},
	}); err != nil {
		t.Fatal(err)
	}
	if opened := <-server.sourceOpen; !opened.GetAccepted() || opened.GetError() != nil {
		t.Fatalf("unexpected source open result: %v", opened)
	}
	if data := <-server.sourceData; data.GetSequence() != 1 || len(data.GetItems()) != 1 {
		t.Fatalf("unexpected source data: %v", data)
	}
	if closed := <-server.sourceClose; closed.GetFinalSequence() != 1 {
		t.Fatalf("unexpected source close: %v", closed)
	}
	closeAckFrame, err := work.Recv()
	closeAck := closeAckFrame.GetTerminalAcknowledgement()
	if err != nil || closeAck.GetWorkId() != "" || closeAck.GetStreamId() != "source-1" ||
		closeAck.GetTerminalDigest() != testTerminalDigest(&SourceClose{StreamId: "source-1", FinalSequence: 1}) {
		t.Fatalf("source terminal acknowledgement failed: frame=%v error=%v", closeAckFrame, err)
	}
	_ = work.CloseSend()
}

func TestProcessBootstrapPreservesTrustedSpecInputs(t *testing.T) {
	bootstrap := &AdapterProcessBootstrap{
		FormatVersion: 2,
		RuntimeTarget: "unix:///run/xgc2/adapter/runtime.sock",
		Registration: &RegisterRequest{
			InstanceId: testInstanceID, ProcessGeneration: testProcessGeneration,
			DefinitionId: "test-adapter", DefinitionDigest: strings.Repeat("d", 64),
			BuildDigest: strings.Repeat("e", 64), ManifestDigest: strings.Repeat("f", 64),
			RuntimeLinkProtocolVersion: 2, SupportedCapabilities: []*CapabilityContract{testCapability()},
		},
		InitialSpec: &AdapterInstanceSpec{
			InstanceId: testInstanceID, ProcessGeneration: testProcessGeneration,
			Revision: 4, SpecDigest: strings.Repeat("b", 64),
			Scope: &ScopeReference{
				Kind: "tenant-context", Key: "sha256:canonical-scope",
				Attributes: map[string]string{"tenant": "tenant-a", "site": "site-1"},
			},
			Secrets: []*SecretReference{{
				Name: "credential", Reference: "secret://credential", Version: "sha256:secret-version",
			}},
		},
	}
	encoded, err := proto.Marshal(bootstrap)
	if err != nil {
		t.Fatal(err)
	}
	var decoded AdapterProcessBootstrap
	if err := proto.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.GetInitialSpec().GetScope().GetAttributes()["tenant"] != "tenant-a" ||
		decoded.GetInitialSpec().GetSecrets()[0].GetVersion() != "sha256:secret-version" {
		t.Fatalf("bootstrap lost canonical scope attributes or secret version: %v", &decoded)
	}
}

func TestOperationAndSourceFramesCarrySafetyAndBackpressure(t *testing.T) {
	operation := &OperationRequest{
		Context: &WorkContext{
			WorkId: "operation-1", CapabilityId: testCapabilityID, EndpointId: "command", SpecRevision: 4,
			ContractVersion: testContractVersion, ContractDigest: testContractDigest,
			Deadline:       &Deadline{DeadlineUnixNanos: time.Now().Add(time.Second).UnixNano(), TtlMs: 1000},
			IdempotencyKey: "operation-key", RequestDigest: strings.Repeat("a", 64),
		},
		Input: testPayload("command"),
	}
	encoded, err := proto.Marshal(operation)
	if err != nil {
		t.Fatal(err)
	}
	var decoded OperationRequest
	if err := proto.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.GetContext().GetDeadline().GetTtlMs() == 0 || decoded.GetContext().GetIdempotencyKey() == "" ||
		decoded.GetContext().GetContractVersion() != testContractVersion || decoded.GetContext().GetContractDigest() != testContractDigest {
		t.Fatalf("operation lost deadline, idempotency, or contract identity: %v", &decoded)
	}

	open := &SourceOpenRequest{
		Context: &WorkContext{
			WorkId: "stream-1", CapabilityId: testCapabilityID, EndpointId: "events", SpecRevision: 4,
			ContractVersion: testContractVersion, ContractDigest: testContractDigest,
		},
		InitialCredit: &FlowControl{Messages: 8, Bytes: 4096},
	}
	accepted := &SourceOpenResult{StreamId: "stream-1", Accepted: true}
	data := &SourceData{StreamId: "stream-1", Sequence: 1, Items: [][]byte{[]byte("typed-item")}}
	credit := &SourceCredit{StreamId: "stream-1", Grant: &FlowControl{Messages: 4, Bytes: 2048}, AcknowledgedSequence: 3}
	if open.GetInitialCredit().GetMessages() == 0 || open.GetInitialCredit().GetBytes() == 0 ||
		credit.GetGrant().GetMessages() == 0 || credit.GetGrant().GetBytes() == 0 || len(data.GetItems()) != 1 {
		t.Fatalf("source frames require independent message and byte credit: open=%v credit=%v", open, credit)
	}
	if open.GetContext().GetContractVersion() != testContractVersion || open.GetContext().GetContractDigest() != testContractDigest {
		t.Fatalf("source open omitted contract identity: %v", open)
	}
	if !accepted.GetAccepted() || accepted.GetInitialPayload() != nil || accepted.GetError() != nil {
		t.Fatalf("empty successful source open is not explicit: %v", accepted)
	}
}

func TestRuntimeLinkV2ExposesOnlyDirectionalSourceFrames(t *testing.T) {
	service := File_xgc_adapter_v1_adapter_proto.Services().ByName("AdapterRuntimeLinkService")
	if service == nil || service.Methods().Len() != 3 ||
		service.Methods().Get(0).Name() != "Register" ||
		service.Methods().Get(1).Name() != "Control" ||
		service.Methods().Get(2).Name() != "Work" {
		t.Fatalf("Runtime Link v2 RPC surface changed: %v", service)
	}

	registrationFields := (&RegisterRequest{}).ProtoReflect().Descriptor().Fields()
	requestVersion := registrationFields.ByName("runtime_link_protocol_version")
	registrationResponseFields := (&RegisterResponse{}).ProtoReflect().Descriptor().Fields()
	responseVersion := registrationResponseFields.ByName("runtime_link_protocol_version")
	if requestVersion == nil || responseVersion == nil ||
		requestVersion.Cardinality() == protoreflect.Repeated || responseVersion.Cardinality() == protoreflect.Repeated ||
		registrationFields.ByName("supported_protocol_versions") != nil ||
		registrationResponseFields.ByName("selected_protocol_version") != nil {
		t.Fatal("Runtime Link v2 registration must use one exact protocol version without negotiation fields")
	}

	interaction := InteractionMode(0).Descriptor()
	if interaction.Values().Len() != 4 ||
		interaction.Values().ByName("INTERACTION_MODE_STREAM_SOURCE") == nil ||
		interaction.Values().ByName("INTERACTION_MODE_STREAM_SINK") != nil ||
		interaction.Values().ByName("INTERACTION_MODE_STREAM_DUPLEX") != nil {
		t.Fatalf("unexpected interaction modes: %v", interaction.Values())
	}

	requestFields := (&WorkRequest{}).ProtoReflect().Descriptor().Fields()
	assertDescriptorFieldNames(t, requestFields, []protoreflect.Name{
		"header", "attach", "unary_response", "operation_event", "source_open_result",
		"source_data", "source_close", "protocol_error",
	})
	for _, field := range []protoreflect.Name{"source_open_result", "source_data", "source_close"} {
		if requestFields.ByName(field) == nil {
			t.Fatalf("Adapter-to-Host WorkRequest omitted %s", field)
		}
	}
	for _, field := range []protoreflect.Name{"source_open_request", "source_credit"} {
		if requestFields.ByName(field) != nil {
			t.Fatalf("Adapter-to-Host WorkRequest illegally exposes %s", field)
		}
	}

	responseFields := (&WorkResponse{}).ProtoReflect().Descriptor().Fields()
	assertDescriptorFieldNames(t, responseFields, []protoreflect.Name{
		"header", "unary_request", "operation_request", "source_open_request", "source_credit",
		"cancellation", "protocol_error", "terminal_acknowledgement",
	})
	for _, field := range []protoreflect.Name{"source_open_request", "source_credit", "cancellation", "terminal_acknowledgement"} {
		if responseFields.ByName(field) == nil {
			t.Fatalf("Host-to-Adapter WorkResponse omitted %s", field)
		}
	}
	for _, field := range []protoreflect.Name{"source_open_result", "source_data", "source_close"} {
		if responseFields.ByName(field) != nil {
			t.Fatalf("Host-to-Adapter WorkResponse illegally exposes %s", field)
		}
	}
	if requestFields.ByName("terminal_acknowledgement") != nil {
		t.Fatal("Adapter-to-Host WorkRequest illegally exposes terminal_acknowledgement")
	}

	sourceOpen := (&SourceOpenRequest{}).ProtoReflect().Descriptor()
	assertDescriptorFieldNames(t, sourceOpen.Fields(), []protoreflect.Name{"context", "initial_credit"})
	if sourceOpen.Fields().ByName("stream_id") != nil {
		t.Fatal("SourceOpenRequest duplicates the stream identity already owned by context.work_id")
	}

	cancellation := (&WorkCancellation{}).ProtoReflect().Descriptor()
	cancelWorkID := cancellation.Fields().ByName("work_id")
	cancelStreamID := cancellation.Fields().ByName("stream_id")
	cancelReason := cancellation.Fields().ByName("reason")
	if cancellation.Fields().Len() != 3 || cancellation.Oneofs().Len() != 1 ||
		cancellation.Oneofs().Get(0).Name() != "identity" ||
		cancelWorkID == nil || cancelStreamID == nil || cancelReason == nil ||
		cancelWorkID.ContainingOneof() != cancellation.Oneofs().Get(0) ||
		cancelStreamID.ContainingOneof() != cancellation.Oneofs().Get(0) ||
		cancelReason.ContainingOneof() != nil {
		t.Fatalf("work cancellation must be exact identity oneof plus reason: %v", cancellation)
	}

	acknowledgement := (&TerminalAcknowledgement{}).ProtoReflect().Descriptor()
	if acknowledgement.Fields().Len() != 3 || acknowledgement.Oneofs().Len() != 1 ||
		acknowledgement.Oneofs().Get(0).Name() != "identity" {
		t.Fatalf("terminal acknowledgement shape changed: %v", acknowledgement)
	}
	workID := acknowledgement.Fields().ByName("work_id")
	streamID := acknowledgement.Fields().ByName("stream_id")
	digest := acknowledgement.Fields().ByName("terminal_digest")
	if workID == nil || streamID == nil || digest == nil ||
		workID.ContainingOneof() != acknowledgement.Oneofs().Get(0) ||
		streamID.ContainingOneof() != acknowledgement.Oneofs().Get(0) ||
		digest.ContainingOneof() != nil {
		t.Fatalf("terminal acknowledgement must be exact identity oneof plus digest: %v", acknowledgement.Fields())
	}
}

func assertDescriptorFieldNames(t *testing.T, fields protoreflect.FieldDescriptors, expected []protoreflect.Name) {
	t.Helper()
	if fields.Len() != len(expected) {
		t.Fatalf("descriptor fields changed: got %v, want %v", fields, expected)
	}
	for index, name := range expected {
		if fields.Get(index).Name() != name {
			t.Fatalf("descriptor field %d=%s, want %s", index, fields.Get(index).Name(), name)
		}
	}
}

func TestAdapterRuntimeContractHasNoDomainLeakage(t *testing.T) {
	file := File_xgc_adapter_v1_adapter_proto
	assertDomainNeutralDescriptorName(t, "file package", string(file.Package()))
	for index := 0; index < file.Imports().Len(); index++ {
		dependency := file.Imports().Get(index)
		assertDomainNeutralDescriptorName(t, "import", dependency.Path())
	}

	var inspectEnums func(protoreflect.EnumDescriptors)
	inspectEnums = func(enums protoreflect.EnumDescriptors) {
		for index := 0; index < enums.Len(); index++ {
			enum := enums.Get(index)
			assertDomainNeutralDescriptorName(t, "enum", string(enum.FullName()))
			for valueIndex := 0; valueIndex < enum.Values().Len(); valueIndex++ {
				value := enum.Values().Get(valueIndex)
				assertDomainNeutralDescriptorName(t, "enum value", string(value.FullName()))
			}
		}
	}
	var inspectMessages func(protoreflect.MessageDescriptors)
	inspectMessages = func(messages protoreflect.MessageDescriptors) {
		for index := 0; index < messages.Len(); index++ {
			message := messages.Get(index)
			assertDomainNeutralDescriptorName(t, "message", string(message.FullName()))
			for fieldIndex := 0; fieldIndex < message.Fields().Len(); fieldIndex++ {
				field := message.Fields().Get(fieldIndex)
				assertDomainNeutralDescriptorName(t, "field", string(field.FullName()))
			}
			for oneofIndex := 0; oneofIndex < message.Oneofs().Len(); oneofIndex++ {
				oneof := message.Oneofs().Get(oneofIndex)
				assertDomainNeutralDescriptorName(t, "oneof", string(oneof.FullName()))
			}
			inspectEnums(message.Enums())
			inspectMessages(message.Messages())
		}
	}
	inspectEnums(file.Enums())
	inspectMessages(file.Messages())
	for serviceIndex := 0; serviceIndex < file.Services().Len(); serviceIndex++ {
		service := file.Services().Get(serviceIndex)
		assertDomainNeutralDescriptorName(t, "service", string(service.FullName()))
		for methodIndex := 0; methodIndex < service.Methods().Len(); methodIndex++ {
			method := service.Methods().Get(methodIndex)
			assertDomainNeutralDescriptorName(t, "RPC", string(method.FullName()))
		}
	}
}

func assertDomainNeutralDescriptorName(t *testing.T, kind, name string) {
	t.Helper()
	lower := strings.ToLower(name)
	for _, token := range []string{
		"robot", "profile", "gateway", "swarm", "workflow", "automation",
		"telemetry", "channel", "experiment", "asset", "panel",
	} {
		if strings.Contains(lower, token) {
			t.Fatalf("Adapter Runtime %s %q contains domain token %q", kind, name, token)
		}
	}
	for _, token := range []string{"run_", "_run", "ros_", "ros1", "ros2", "ui_", "_ui", ".ui", "user_interface"} {
		if strings.Contains(lower, token) {
			t.Fatalf("Adapter Runtime %s %q contains domain token %q", kind, name, token)
		}
	}
	short := lower
	if separator := strings.LastIndexByte(short, '.'); separator >= 0 {
		short = short[separator+1:]
	}
	if strings.HasPrefix(short, "ros") || strings.HasPrefix(short, "ui") ||
		(strings.HasPrefix(short, "run") && !strings.HasPrefix(short, "runtime")) {
		t.Fatalf("Adapter Runtime %s %q contains a domain prefix", kind, name)
	}
}
