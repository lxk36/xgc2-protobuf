#!/usr/bin/env bash
set -euo pipefail

root_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
tool_dir="${root_dir}/.tools/bin"

mkdir -p "${tool_dir}"

GOBIN="${tool_dir}" go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.36.10
GOBIN="${tool_dir}" go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.5.1

echo "Installed pinned Go protobuf plugins in ${tool_dir}"
