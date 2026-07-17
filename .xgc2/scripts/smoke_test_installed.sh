#!/usr/bin/env bash

set -euo pipefail

package_name="xgc2-protobuf-dev"
schema_root="/usr/share/xgc2-protobuf"

dpkg -s "${package_name}" >/dev/null
test -f "${schema_root}/proto/xgc/v1/message.proto"
test -f "${schema_root}/proto/xgc/adapter/v1/adapter.proto"
test -f "${schema_root}/proto/xgc/robot/v1/message.proto"
test -f "${schema_root}/descriptors/xgc2-protocols.pb"
test -f "${schema_root}/registry/messages.yaml"
test -f "${schema_root}/registry/registry.json"
test ! -e "${schema_root}/profiles"
test -f /usr/share/cmake/xgc2_protobuf/xgc2_protobufConfig.cmake
test -f /usr/share/pkgconfig/xgc2-protobuf.pc

if find "${schema_root}" -type f \( \
    -name '*.pb.h' -o \
    -name '*.pb.cc' -o \
    -name '*_pb2.py' -o \
    -name '*.go' \
  \) -print -quit | grep -q .; then
  echo "installed package contains generated language bindings" >&2
  exit 1
fi

python3 - <<'PY'
import json
from pathlib import Path

registry = json.loads(Path("/usr/share/xgc2-protobuf/registry/registry.json").read_text())
if not registry.get("messages"):
    raise SystemExit("installed registry has no messages")
PY

protoc \
  -I "${schema_root}/proto" \
  --descriptor_set_out=/tmp/xgc2-protobuf-smoke.pb \
  xgc/v1/message.proto \
  xgc/adapter/v1/adapter.proto \
  xgc/robot/v1/message.proto
test -s /tmp/xgc2-protobuf-smoke.pb

probe_dir="${XGC2_PROTOBUF_SMOKE_DIR:-$(mktemp -d -t xgc2-protobuf-smoke-XXXXXX)}"
mkdir -p "${probe_dir}"
cat > "${probe_dir}/CMakeLists.txt" <<'CMAKE'
cmake_minimum_required(VERSION 3.10)
project(xgc2_protobuf_probe LANGUAGES NONE)

find_package(xgc2_protobuf REQUIRED CONFIG)
if(NOT TARGET xgc2_protobuf::schemas)
  message(FATAL_ERROR "xgc2_protobuf::schemas target is missing")
endif()
if(NOT EXISTS "${XGC2_PROTOBUF_PROTO_ROOT}/xgc/v1/message.proto")
  message(FATAL_ERROR "XGC2_PROTOBUF_PROTO_ROOT is invalid")
endif()
if(NOT EXISTS "${XGC2_PROTOBUF_DESCRIPTOR_SET}")
  message(FATAL_ERROR "XGC2_PROTOBUF_DESCRIPTOR_SET is invalid")
endif()
CMAKE
cmake -S "${probe_dir}" -B "${probe_dir}/build"

test "$(pkg-config --variable=proto_root xgc2-protobuf)" = "${schema_root}/proto"
test "$(pkg-config --variable=descriptor_set xgc2-protobuf)" = \
  "${schema_root}/descriptors/xgc2-protocols.pb"

echo "xgc2-protobuf-dev installed smoke test passed."
