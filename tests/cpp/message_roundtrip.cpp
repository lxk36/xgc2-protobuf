#include <cassert>
#include <iostream>
#include <memory>
#include <string>

#include "xgc/registry/v1/message_registry.hpp"
#include "xgc/semantic/aerial/v1/flight.pb.h"
#include "xgc/v1/message.pb.h"

int main() {
  xgc::semantic::aerial::v1::FlightModeRequest request;
  request.set_mode("OFFBOARD");

  std::string payload;
  assert(request.SerializeToString(&payload));

  const auto* metadata = xgc::registry::v1::findMessage(3111);
  assert(metadata != nullptr);

  xgc::v1::Message envelope;
  envelope.set_robot_id("uav1");
  envelope.set_channel_id("flight.set_mode");
  envelope.set_message_id(metadata->id);
  envelope.set_schema_version(metadata->version);
  envelope.set_schema_fingerprint(metadata->fingerprint);
  envelope.set_encoding(xgc::v1::PAYLOAD_ENCODING_PROTOBUF);
  envelope.set_payload(payload);

  std::unique_ptr<google::protobuf::Message> decoded =
      xgc::registry::v1::newMessage(envelope.message_id());
  assert(decoded != nullptr);
  assert(decoded->ParseFromString(envelope.payload()));

  const auto* mode =
      dynamic_cast<const xgc::semantic::aerial::v1::FlightModeRequest*>(decoded.get());
  assert(mode != nullptr);
  assert(mode->mode() == "OFFBOARD");
  assert(xgc::registry::v1::newMessage(999999) == nullptr);

  std::cout << "cpp roundtrip ok: " << envelope.robot_id() << " " << envelope.channel_id()
            << " " << mode->mode() << std::endl;
  return 0;
}
