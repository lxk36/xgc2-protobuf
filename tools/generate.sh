#!/usr/bin/env bash
set -euo pipefail

root_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
proto_dir="${root_dir}/proto"
generated_dir="${root_dir}/generated"
cpp_out="${generated_dir}/cpp"
go_out="${generated_dir}/go"
python_out="${generated_dir}/python"
descriptor_out="${generated_dir}/descriptors/xgc2-protocols.pb"
adapter_proto="xgc/adapter/v1/adapter.proto"

export PATH="${root_dir}/.tools/bin:${PATH}"

for command in protoc grpc_cpp_plugin protoc-gen-go protoc-gen-go-grpc python3; do
  if ! command -v "${command}" >/dev/null 2>&1; then
    echo "missing required generator: ${command}" >&2
    if [[ "${command}" == protoc-gen-go || "${command}" == protoc-gen-go-grpc ]]; then
      echo "run tools/install-go-plugins.sh once, or install the pinned plugins from Debian" >&2
    fi
    exit 1
  fi
done

if ! python3 -c 'import grpc_tools.protoc, yaml' >/dev/null 2>&1; then
  echo "python grpc_tools and PyYAML are required" >&2
  exit 1
fi

mapfile -t proto_files < <(cd "${proto_dir}" && find xgc -type f -name '*.proto' -print | sort)
if [[ ${#proto_files[@]} -eq 0 ]]; then
  echo "no protobuf sources found" >&2
  exit 1
fi

rm -rf "${cpp_out}" "${go_out}" "${python_out}"
mkdir -p "${cpp_out}" "${go_out}" "${python_out}" "$(dirname "${descriptor_out}")"

(
  cd "${proto_dir}"
  protoc -I . --cpp_out="${cpp_out}" "${proto_files[@]}"
  protoc -I . \
    --grpc_out="${cpp_out}" \
    --plugin=protoc-gen-grpc="$(command -v grpc_cpp_plugin)" \
    "${adapter_proto}"
  protoc -I . \
    --include_imports \
    --descriptor_set_out="${descriptor_out}" \
    "${proto_files[@]}"

  protoc -I . --go_out=paths=source_relative:"${go_out}" "${proto_files[@]}"
  protoc -I . \
    --go-grpc_out=paths=source_relative:"${go_out}" \
    "${adapter_proto}"

  python3 -m grpc_tools.protoc -I . --python_out="${python_out}" "${proto_files[@]}"
  python3 -m grpc_tools.protoc -I . \
    --grpc_python_out="${python_out}" \
    "${adapter_proto}"
)

cp "${root_dir}/packaging/go/go.mod" "${go_out}/go.mod"
cp "${root_dir}/packaging/go/go.sum" "${go_out}/go.sum"

python3 "${root_dir}/tools/generate_registry.py" \
  --registry "${root_dir}/registry/messages.yaml" \
  --descriptors "${descriptor_out}" \
  --cpp-out "${cpp_out}/xgc/registry/v1" \
  --go-out "${go_out}/xgc/registry/v1" \
  --python-out "${python_out}/xgc/registry/v1" \
  --metadata-out "${generated_dir}/registry.json"

python3 "${root_dir}/tools/validate_profiles.py" \
  --registry "${root_dir}/registry/messages.yaml" \
  --profiles "${root_dir}/profiles" \
  --metadata-out "${generated_dir}/profile-registry.json"

gofmt -w "${go_out}/xgc/registry/v1/registry.go"

echo "Generated C++, Go, Python, descriptors, message registry, and profile registry under ${generated_dir}"
