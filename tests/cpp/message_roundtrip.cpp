#include <cassert>
#include <iostream>
#include <memory>
#include <string>

#include "xgc/mavlink/v1/command.pb.h"
#include "xgc/registry/v1/message_registry.hpp"
#include "xgc/v1/message.pb.h"

int main() {
  xgc::mavlink::v1::CommandLongRequest request;
  request.set_command(176);
  request.set_param1(1.0F);
  request.set_param2(6.0F);

  std::string payload;
  assert(request.SerializeToString(&payload));

  const auto* metadata = xgc::registry::v1::findMessage(5001);
  assert(metadata != nullptr);

  xgc::v1::Message envelope;
  envelope.set_robot_id("uav1");
  envelope.set_channel_id("mavlink.command_long");
  envelope.set_message_id(metadata->id);
  envelope.set_schema_version(metadata->version);
  envelope.set_schema_fingerprint(metadata->fingerprint);
  envelope.set_encoding(xgc::v1::PAYLOAD_ENCODING_PROTOBUF);
  envelope.set_payload(payload);

  std::unique_ptr<google::protobuf::Message> decoded =
      xgc::registry::v1::newMessage(envelope.message_id());
  assert(decoded != nullptr);
  assert(decoded->ParseFromString(envelope.payload()));

  const auto* command = dynamic_cast<const xgc::mavlink::v1::CommandLongRequest*>(decoded.get());
  assert(command != nullptr);
  assert(command->command() == 176);
  assert(command->param1() == 1.0F);
  assert(command->param2() == 6.0F);

  std::unique_ptr<google::protobuf::Message> ack = xgc::registry::v1::newMessage(5099);
  assert(dynamic_cast<const xgc::mavlink::v1::CommandAck*>(ack.get()) != nullptr);
  assert(xgc::registry::v1::newMessage(999999) == nullptr);

  std::cout << "cpp roundtrip ok: " << envelope.robot_id() << " " << envelope.channel_id()
            << " command=" << command->command() << std::endl;
  return 0;
}
