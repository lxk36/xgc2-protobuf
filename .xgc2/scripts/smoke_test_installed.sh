#!/usr/bin/env bash

set -euo pipefail

package_name="xgc2-protobuf-dev"
schema_root="/usr/share/xgc2-protobuf"

dpkg -s "${package_name}" >/dev/null
test -f "${schema_root}/proto/xgc/v1/message.proto"
test -f "${schema_root}/proto/xgc/adapter/v1/adapter.proto"
test -f "${schema_root}/descriptors/xgc2-protocols.pb"
test -f "${schema_root}/registry/messages.yaml"
test -f "${schema_root}/registry/registry.json"
test -d "${schema_root}/profiles"
test -f "${schema_root}/profiles/schema/adapter-profile-v1.schema.json"
test -f "${schema_root}/profiles/ros1/px4-multirotor-ros1-v1.yaml"
test -f "${schema_root}/profiles/ros1/scout-mini-ros1-v1.yaml"
test -f "${schema_root}/profiles/registry.json"
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
import hashlib
from pathlib import Path

registry = json.loads(Path("/usr/share/xgc2-protobuf/registry/registry.json").read_text())
if not registry.get("messages"):
    raise SystemExit("installed registry has no messages")

profiles_root = Path("/usr/share/xgc2-protobuf/profiles")
profile_registry = json.loads((profiles_root / "registry.json").read_text())
if profile_registry.get("digest_algorithm") != "sha256-raw-bytes":
    raise SystemExit("installed profile registry has an unexpected digest algorithm")
profiles = profile_registry.get("profiles")
if not isinstance(profiles, list) or not profiles:
    raise SystemExit("installed profile registry has no profiles")

registered_files = set()
for profile in profiles:
    if not isinstance(profile, dict) or not profile.get("file") or not profile.get("profile_id"):
        raise SystemExit("installed profile registry contains an invalid entry")
    relative_path = Path(profile["file"])
    if relative_path.is_absolute() or ".." in relative_path.parts:
        raise SystemExit("installed profile registry contains an unsafe path")
    normalized_path = relative_path.as_posix()
    if normalized_path in registered_files:
        raise SystemExit("installed profile registry contains a duplicate file")
    registered_files.add(normalized_path)
    installed_path = profiles_root / relative_path
    if not installed_path.is_file():
        raise SystemExit("installed profile file is missing: " + normalized_path)
    actual = hashlib.sha256(installed_path.read_bytes()).hexdigest()
    if actual != profile["digest"]:
        raise SystemExit("installed profile digest mismatch for " + profile["profile_id"])

installed_files = {
    path.relative_to(profiles_root).as_posix()
    for path in profiles_root.rglob("*.yaml")
}
if registered_files != installed_files:
    raise SystemExit("installed profile registry does not match installed profile files")
PY

protoc \
  -I "${schema_root}/proto" \
  --descriptor_set_out=/tmp/xgc2-protobuf-smoke.pb \
  xgc/v1/message.proto \
  xgc/adapter/v1/adapter.proto
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
test "$(pkg-config --variable=profile_registry xgc2-protobuf)" = \
  "${schema_root}/profiles/registry.json"

echo "xgc2-protobuf-dev installed smoke test passed."
