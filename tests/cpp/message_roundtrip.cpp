#include <cassert>
#include <iostream>
#include <memory>
#include <string>

#include "xgc/registry/v1/message_registry.hpp"
#include "xgc/robot/v1/message.pb.h"
#include "xgc/semantic/aerial/v1/control.pb.h"
#include "xgc/semantic/ground/v1/control.pb.h"
#include "xgc/v1/message.pb.h"

int main() {
  const auto* empty_metadata = xgc::registry::v1::findMessage(1);
  assert(empty_metadata != nullptr);
  assert(empty_metadata->version == 1u);
  assert(empty_metadata->fingerprint == 11009224659857530918ULL);
  assert(std::string(empty_metadata->full_name) == "xgc.v1.Empty");
  assert(dynamic_cast<xgc::v1::Empty*>(xgc::registry::v1::newMessage(1).get()) != nullptr);

  const auto* spec_metadata = xgc::registry::v1::findMessage(4001);
  assert(spec_metadata != nullptr);
  assert(spec_metadata->version == 2u);
  assert(spec_metadata->fingerprint == 1932893837531035663ULL);
  assert(std::string(spec_metadata->full_name) == "xgc.robot.v1.RobotAdapterSpec");
  assert(dynamic_cast<xgc::robot::v1::RobotAdapterSpec*>(
             xgc::registry::v1::newMessage(4001).get()) != nullptr);

  const auto* routed_metadata = xgc::registry::v1::findMessage(4002);
  assert(routed_metadata != nullptr);
  assert(routed_metadata->version == 1u);
  assert(routed_metadata->fingerprint == 17079265246794908236ULL);
  assert(std::string(routed_metadata->full_name) == "xgc.robot.v1.RobotMessage");
  assert(dynamic_cast<xgc::robot::v1::RobotMessage*>(
             xgc::registry::v1::newMessage(4002).get()) != nullptr);

  xgc::semantic::aerial::v1::ModeRequest request;
  request.set_mode("OFFBOARD");

  std::string payload;
  assert(request.SerializeToString(&payload));

  const auto* metadata = xgc::registry::v1::findMessage(3202);
  assert(metadata != nullptr);

  xgc::v1::Message envelope;
  envelope.set_sequence(1);
  envelope.mutable_payload()->mutable_schema()->set_message_id(metadata->id);
  envelope.mutable_payload()->mutable_schema()->set_type_name(metadata->full_name);
  envelope.mutable_payload()->mutable_schema()->set_schema_version(metadata->version);
  envelope.mutable_payload()->mutable_schema()->set_schema_fingerprint(metadata->fingerprint);
  envelope.mutable_payload()->set_encoding(xgc::v1::PAYLOAD_ENCODING_PROTOBUF);
  envelope.mutable_payload()->set_value(payload);

  xgc::robot::v1::RobotMessage routed;
  routed.set_robot_id("uav1");
  routed.set_channel_id("operation.mode");
  *routed.mutable_message() = envelope;

  std::unique_ptr<google::protobuf::Message> decoded =
      xgc::registry::v1::newMessage(envelope.payload().schema().message_id());
  assert(decoded != nullptr);
  assert(decoded->ParseFromString(envelope.payload().value()));

  const auto* mode = dynamic_cast<const xgc::semantic::aerial::v1::ModeRequest*>(decoded.get());
  assert(mode != nullptr);
  assert(mode->mode() == "OFFBOARD");

  assert(dynamic_cast<xgc::semantic::aerial::v1::ArmRequest*>(
             xgc::registry::v1::newMessage(3201).get()) != nullptr);
  assert(dynamic_cast<xgc::semantic::aerial::v1::AutopilotRebootRequest*>(
             xgc::registry::v1::newMessage(3203).get()) != nullptr);
  const auto* ground_intent_metadata = xgc::registry::v1::findMessage(3204);
  assert(ground_intent_metadata != nullptr);
  assert(ground_intent_metadata->fingerprint == 13602409479439522314ULL);
  assert(dynamic_cast<xgc::semantic::ground::v1::MotionIntentRequest*>(
             xgc::registry::v1::newMessage(3204).get()) != nullptr);
  assert(xgc::registry::v1::newMessage(5001) == nullptr);

  std::cout << "cpp roundtrip ok: " << routed.robot_id() << " " << routed.channel_id()
            << " mode=" << mode->mode() << std::endl;
  return 0;
}
