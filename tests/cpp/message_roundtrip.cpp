#include <cassert>
#include <iostream>
#include <memory>
#include <string>

#include "xgc/registry/v1/message_registry.hpp"
#include "xgc/semantic/aerial/v1/control.pb.h"
#include "xgc/v1/message.pb.h"

int main() {
  xgc::semantic::aerial::v1::ModeRequest request;
  request.set_mode("OFFBOARD");

  std::string payload;
  assert(request.SerializeToString(&payload));

  const auto* metadata = xgc::registry::v1::findMessage(3202);
  assert(metadata != nullptr);

  xgc::v1::Message envelope;
  envelope.set_robot_id("uav1");
  envelope.set_channel_id("operation.mode");
  envelope.set_message_id(metadata->id);
  envelope.set_payload(payload);

  std::unique_ptr<google::protobuf::Message> decoded =
      xgc::registry::v1::newMessage(envelope.message_id());
  assert(decoded != nullptr);
  assert(decoded->ParseFromString(envelope.payload()));

  const auto* mode = dynamic_cast<const xgc::semantic::aerial::v1::ModeRequest*>(decoded.get());
  assert(mode != nullptr);
  assert(mode->mode() == "OFFBOARD");

  assert(dynamic_cast<xgc::semantic::aerial::v1::ArmRequest*>(
             xgc::registry::v1::newMessage(3201).get()) != nullptr);
  assert(dynamic_cast<xgc::semantic::aerial::v1::AutopilotRebootRequest*>(
             xgc::registry::v1::newMessage(3203).get()) != nullptr);
  assert(xgc::registry::v1::newMessage(5001) == nullptr);

  std::cout << "cpp roundtrip ok: " << envelope.robot_id() << " " << envelope.channel_id()
            << " mode=" << mode->mode() << std::endl;
  return 0;
}
