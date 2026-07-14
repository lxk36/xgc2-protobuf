#!/usr/bin/env bash

set -euo pipefail

against_ref="${1:-}"
if [[ -z "${against_ref}" ]]; then
  echo "usage: $0 <git-ref>" >&2
  exit 2
fi

product_version() {
  awk -F': *' '/^version:[[:space:]]*/ {print $2; exit}'
}

protocol_epoch() {
  local version="$1"
  local major minor
  version="${version%%~*}"
  version="${version%%-*}"
  IFS=. read -r major minor _ <<<"${version}"
  if [[ ! "${major}" =~ ^[0-9]+$ || ! "${minor}" =~ ^[0-9]+$ ]]; then
    echo "invalid protocol product version: $1" >&2
    exit 2
  fi
  if [[ "${major}" == "0" ]]; then
    printf '0.%s\n' "${minor}"
  else
    printf '%s\n' "${major}"
  fi
}

current_version="$(product_version < .xgc2/product.yml)"
base_version="$(git show "${against_ref}:.xgc2/product.yml" | product_version)"
current_epoch="$(protocol_epoch "${current_version}")"
base_epoch="$(protocol_epoch "${base_version}")"

if [[ "${current_epoch}" != "${base_epoch}" ]]; then
  echo "Protocol epoch changed ${base_epoch} -> ${current_epoch}; intentional breaking check reset."
  exit 0
fi

buf breaking --against ".git#ref=${against_ref}"
