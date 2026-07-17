#!/usr/bin/env bash
set -euo pipefail

root_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

"${root_dir}/tools/generate.sh"

cp "${root_dir}/packaging/go/go.sum" "${root_dir}/generated/go/go.sum"
cp "${root_dir}/tests/go/registry_test.go" \
  "${root_dir}/generated/go/xgc/registry/v1/registry_test.go"
cp "${root_dir}/tests/go/adapter_runtime_link_test.go" \
  "${root_dir}/generated/go/xgc/adapter/v1/adapter_runtime_link_test.go"
gofmt -w \
  "${root_dir}/generated/go/xgc/adapter/v1/adapter_runtime_link_test.go" \
  "${root_dir}/generated/go/xgc/registry/v1/registry.go" \
  "${root_dir}/generated/go/xgc/registry/v1/registry_test.go"

(
  cd "${root_dir}/generated/go"
  GOPROXY="${XGC2_GO_PROXY:-off}" go test ./...
)

PYTHONPATH="${root_dir}/generated/python" \
  python3 "${root_dir}/tests/python/message_roundtrip.py"

mapfile -t cpp_sources < <(
  find "${root_dir}/generated/cpp" -type f \
    \( -name '*.pb.cc' -o -name 'message_registry.cpp' \) \
    ! -name '*.grpc.pb.cc' -print | sort
)
mkdir -p "${root_dir}/generated/tests"
read -r -a protobuf_flags <<<"$(pkg-config --cflags --libs protobuf)"
g++ -std=c++14 -O0 -g \
  -I"${root_dir}/generated/cpp" \
  "${root_dir}/tests/cpp/message_roundtrip.cpp" \
  "${cpp_sources[@]}" \
  "${protobuf_flags[@]}" \
  -pthread \
  -o "${root_dir}/generated/tests/cpp-message-roundtrip"
"${root_dir}/generated/tests/cpp-message-roundtrip"

echo "C++, Go, and Python protocol smoke tests passed"
