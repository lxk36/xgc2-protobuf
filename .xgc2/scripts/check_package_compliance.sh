#!/usr/bin/env bash

set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "${repo_root}"

bash -n .xgc2/scripts/*.sh tools/*.sh

nested_git="$(
  find . \
    -path ./.git -prune -o \
    -path ./.ci -prune -o \
    -path ./generated -prune -o \
    -name .git -print
)"
if [[ -n "${nested_git}" ]]; then
  echo "Nested .git directory found." >&2
  echo "${nested_git}" >&2
  exit 1
fi

if git ls-files 2>/dev/null | grep -E '(^|/)(build|devel|install|\.ci|generated)(/|$)' >/dev/null; then
  echo "Generated build artifacts are tracked." >&2
  git ls-files | grep -E '(^|/)(build|devel|install|\.ci|generated)(/|$)' >&2
  exit 1
fi

required_files=(
  README.md
  buf.yaml
  proto/xgc/v1/message.proto
  proto/xgc/adapter/v1/adapter.proto
  registry/messages.yaml
  profiles/ros1/mobile-base-twist-v1.yaml
  tools/generate_registry.py
  tools/validate_profiles.py
  .github/workflows/ci.yml
  .github/workflows/release.yml
  .xgc2/product.yml
  .xgc2/cmake/xgc2_protobufConfig.cmake
  .xgc2/pkgconfig/xgc2-protobuf.pc
  .xgc2/scripts/build_deb.sh
  .xgc2/scripts/check_package_compliance.sh
  .xgc2/scripts/smoke_test_installed.sh
  .xgc2/scripts/xgc2_artifact_manifest.py
)

for file in "${required_files[@]}"; do
  if [[ ! -f "${file}" ]]; then
    echo "Missing required file: ${file}" >&2
    exit 1
  fi
done

if ! grep -q '^id: xgc2-protobuf$' .xgc2/product.yml ||
   ! grep -q '^version: 0.1.0-2$' .xgc2/product.yml ||
   ! grep -q '^kind: toolchain-apt$' .xgc2/product.yml; then
  echo "product metadata identity/version/kind is inconsistent" >&2
  exit 1
fi

if [[ "$(find proto -type f -name '*.proto' | wc -l)" -eq 0 ]]; then
  echo "No protobuf schemas found." >&2
  exit 1
fi

python3 tools/validate_profiles.py \
  --registry registry/messages.yaml \
  --profiles profiles

echo "xgc2-protobuf-dev package compliance checks passed."
