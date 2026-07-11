#!/usr/bin/env python3

import argparse
import hashlib
import json
from pathlib import Path
from typing import Dict, Iterable, List, Tuple

import yaml
from google.protobuf import descriptor_pb2


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Generate XGC2 message registries")
    parser.add_argument("--registry", required=True)
    parser.add_argument("--descriptors", required=True)
    parser.add_argument("--cpp-out", required=True)
    parser.add_argument("--go-out", required=True)
    parser.add_argument("--python-out", required=True)
    parser.add_argument("--metadata-out", required=True)
    return parser.parse_args()


def load_descriptors(path: Path):
    descriptor_set = descriptor_pb2.FileDescriptorSet()
    descriptor_set.ParseFromString(path.read_bytes())
    files = {item.name: item for item in descriptor_set.file}
    messages = {}
    for file_descriptor in descriptor_set.file:
        package = file_descriptor.package
        for message in file_descriptor.message_type:
            full_name = ".".join(part for part in (package, message.name) if part)
            messages[full_name] = (file_descriptor, message)
    return files, messages


def descriptor_fingerprint(
    full_name: str,
    file_descriptor: descriptor_pb2.FileDescriptorProto,
    files: Dict[str, descriptor_pb2.FileDescriptorProto],
) -> int:
    closure = set()

    def visit(file_name: str) -> None:
        if file_name in closure:
            return
        closure.add(file_name)
        for dependency in files[file_name].dependency:
            if dependency in files:
                visit(dependency)

    visit(file_descriptor.name)
    digest = hashlib.sha256(full_name.encode("utf-8"))
    for file_name in sorted(closure):
        digest.update(file_name.encode("utf-8"))
        digest.update(files[file_name].SerializeToString(deterministic=True))
    return int.from_bytes(digest.digest()[:8], byteorder="big", signed=False)


def load_registry(path: Path, files, descriptors) -> Tuple[List[dict], int]:
    document = yaml.safe_load(path.read_text(encoding="utf-8"))
    if document.get("version") != 1:
        raise ValueError("registry version must be 1")

    reserved_ids = set()
    for raw in document.get("reserved_ids", []):
        message_id = int(raw)
        if message_id <= 0 or message_id in reserved_ids:
            raise ValueError(
                "reserved message IDs must be positive and unique: {}".format(message_id)
            )
        reserved_ids.add(message_id)

    entries = []
    seen_ids = set()
    seen_types = set()
    for raw in document.get("messages", []):
        message_id = int(raw["id"])
        full_name = str(raw["type"])
        version = int(raw["version"])
        roles = [str(role) for role in raw.get("roles", [])]
        if message_id <= 0 or message_id in seen_ids:
            raise ValueError("message ID must be positive and unique: {}".format(message_id))
        if message_id in reserved_ids:
            raise ValueError("message ID is reserved and cannot be reused: {}".format(message_id))
        if full_name in seen_types:
            raise ValueError("message type registered more than once: {}".format(full_name))
        if full_name not in descriptors:
            raise ValueError("registered protobuf message does not exist: {}".format(full_name))
        if version <= 0:
            raise ValueError("schema version must be positive for {}".format(full_name))

        file_descriptor, message_descriptor = descriptors[full_name]
        if message_descriptor.nested_type:
            raise ValueError("registered messages with nested declarations are not supported yet: {}".format(full_name))
        fingerprint = descriptor_fingerprint(full_name, file_descriptor, files)
        entries.append(
            {
                "id": message_id,
                "type": full_name,
                "version": version,
                "roles": roles,
                "fingerprint": fingerprint,
                "file": file_descriptor.name,
                "go_package": file_descriptor.options.go_package,
            }
        )
        seen_ids.add(message_id)
        seen_types.add(full_name)

    entries.sort(key=lambda item: item["id"])
    registry_digest = hashlib.sha256()
    for entry in entries:
        registry_digest.update(
            "{id}:{type}:{version}:{fingerprint}\n".format(**entry).encode("utf-8")
        )
    registry_fingerprint = int.from_bytes(
        registry_digest.digest()[:8], byteorder="big", signed=False
    )
    return entries, registry_fingerprint


def cpp_class(full_name: str) -> str:
    parts = full_name.split(".")
    return "::" + "::".join(parts)


def generate_cpp(entries: Iterable[dict], registry_fingerprint: int, output: Path) -> None:
    output.mkdir(parents=True, exist_ok=True)
    header = output / "message_registry.hpp"
    source = output / "message_registry.cpp"
    includes = sorted({entry["file"].replace(".proto", ".pb.h") for entry in entries})

    header.write_text(
        """#pragma once

#include <cstdint>
#include <memory>

#include <google/protobuf/message.h>

namespace xgc {
namespace registry {
namespace v1 {

struct MessageMetadata {
  std::uint32_t id;
  std::uint32_t version;
  std::uint64_t fingerprint;
  const char* full_name;
  const char* roles;
};

std::uint64_t registryFingerprint();
const MessageMetadata* findMessage(std::uint32_t id);
std::unique_ptr<::google::protobuf::Message> newMessage(std::uint32_t id);

}  // namespace v1
}  // namespace registry
}  // namespace xgc
""",
        encoding="utf-8",
    )

    lines = [
        '#include "xgc/registry/v1/message_registry.hpp"',
        "",
    ]
    lines.extend('#include "{}"'.format(item) for item in includes)
    lines.extend(
        [
            "",
            "namespace xgc {",
            "namespace registry {",
            "namespace v1 {",
            "namespace {",
            "",
            "const MessageMetadata kMessages[] = {",
        ]
    )
    for entry in entries:
        roles = ",".join(entry["roles"])
        lines.append(
            '  {{{}u, {}u, {}ULL, "{}", "{}"}},'.format(
                entry["id"],
                entry["version"],
                entry["fingerprint"],
                entry["type"],
                roles,
            )
        )
    lines.extend(
        [
            "};",
            "",
            "}  // namespace",
            "",
            "std::uint64_t registryFingerprint() {",
            "  return {}ULL;".format(registry_fingerprint),
            "}",
            "",
            "const MessageMetadata* findMessage(std::uint32_t id) {",
            "  for (const auto& metadata : kMessages) {",
            "    if (metadata.id == id) {",
            "      return &metadata;",
            "    }",
            "  }",
            "  return nullptr;",
            "}",
            "",
            "std::unique_ptr<::google::protobuf::Message> newMessage(std::uint32_t id) {",
            "  switch (id) {",
        ]
    )
    for entry in entries:
        lines.append(
            "    case {}u: return std::unique_ptr<::google::protobuf::Message>(new {}());".format(
                entry["id"], cpp_class(entry["type"])
            )
        )
    lines.extend(
        [
            "    default: return nullptr;",
            "  }",
            "}",
            "",
            "}  // namespace v1",
            "}  // namespace registry",
            "}  // namespace xgc",
            "",
        ]
    )
    source.write_text("\n".join(lines), encoding="utf-8")


def go_package_parts(go_package: str) -> Tuple[str, str]:
    if ";" in go_package:
        path, alias = go_package.split(";", 1)
        return path, alias
    path = go_package
    return path, path.rsplit("/", 1)[-1]


def generate_go(entries: Iterable[dict], registry_fingerprint: int, output: Path) -> None:
    output.mkdir(parents=True, exist_ok=True)
    imports = {}
    for entry in entries:
        import_path, alias = go_package_parts(entry["go_package"])
        imports[import_path] = alias

    lines = [
        "// Code generated by tools/generate_registry.py. DO NOT EDIT.",
        "package registryv1",
        "",
        "import (",
        '\t"google.golang.org/protobuf/proto"',
    ]
    for import_path, alias in sorted(imports.items()):
        lines.append('\t{} "{}"'.format(alias, import_path))
    lines.extend(
        [
            ")",
            "",
            "type Metadata struct {",
            "\tID uint32",
            "\tVersion uint32",
            "\tFingerprint uint64",
            "\tFullName string",
            "\tRoles []string",
            "}",
            "",
            "const RegistryFingerprint uint64 = {}".format(registry_fingerprint),
            "",
            "var metadata = map[uint32]Metadata{",
        ]
    )
    for entry in entries:
        roles = ", ".join('"{}"'.format(role) for role in entry["roles"])
        lines.append(
            '\t{}: {{ID: {}, Version: {}, Fingerprint: {}, FullName: "{}", Roles: []string{{{}}}}},'.format(
                entry["id"],
                entry["id"],
                entry["version"],
                entry["fingerprint"],
                entry["type"],
                roles,
            )
        )
    lines.extend(
        [
            "}",
            "",
            "func Lookup(id uint32) (Metadata, bool) {",
            "\titem, ok := metadata[id]",
            "\treturn item, ok",
            "}",
            "",
            "func New(id uint32) (proto.Message, bool) {",
            "\tswitch id {",
        ]
    )
    for entry in entries:
        _, alias = go_package_parts(entry["go_package"])
        class_name = entry["type"].rsplit(".", 1)[-1]
        lines.append("\tcase {}:".format(entry["id"]))
        lines.append("\t\treturn &{}.{}{{}}, true".format(alias, class_name))
    lines.extend(
        [
            "\tdefault:",
            "\t\treturn nil, false",
            "\t}",
            "}",
            "",
        ]
    )
    (output / "registry.go").write_text("\n".join(lines), encoding="utf-8")


def python_module(file_name: str) -> Tuple[str, str]:
    module = file_name.replace(".proto", "_pb2").replace("/", ".")
    package, name = module.rsplit(".", 1)
    return package, name


def generate_python(entries: Iterable[dict], registry_fingerprint: int, output: Path) -> None:
    output.mkdir(parents=True, exist_ok=True)
    modules = {}
    for entry in entries:
        package, module = python_module(entry["file"])
        alias = "{}_{}".format(package.replace(".", "_"), module)
        modules[(package, module)] = alias

    lines = [
        "# Code generated by tools/generate_registry.py. DO NOT EDIT.",
    ]
    for (package, module), alias in sorted(modules.items()):
        lines.append("from {} import {} as {}".format(package, module, alias))
    lines.extend(
        [
            "",
            "REGISTRY_FINGERPRINT = {}".format(registry_fingerprint),
            "",
            "METADATA = {",
        ]
    )
    for entry in entries:
        roles = repr(entry["roles"])
        lines.append(
            '    {}: {{"id": {}, "version": {}, "fingerprint": {}, "full_name": "{}", "roles": {}}},'.format(
                entry["id"],
                entry["id"],
                entry["version"],
                entry["fingerprint"],
                entry["type"],
                roles,
            )
        )
    lines.extend(["}", "", "MESSAGE_TYPES = {"])
    for entry in entries:
        package, module = python_module(entry["file"])
        alias = modules[(package, module)]
        class_name = entry["type"].rsplit(".", 1)[-1]
        lines.append("    {}: {}.{},".format(entry["id"], alias, class_name))
    lines.extend(
        [
            "}",
            "",
            "def new_message(message_id):",
            "    message_type = MESSAGE_TYPES.get(message_id)",
            "    return message_type() if message_type is not None else None",
            "",
        ]
    )
    (output / "message_registry.py").write_text("\n".join(lines), encoding="utf-8")


def main() -> None:
    args = parse_args()
    files, descriptors = load_descriptors(Path(args.descriptors))
    entries, registry_fingerprint = load_registry(Path(args.registry), files, descriptors)

    generate_cpp(entries, registry_fingerprint, Path(args.cpp_out))
    generate_go(entries, registry_fingerprint, Path(args.go_out))
    generate_python(entries, registry_fingerprint, Path(args.python_out))

    metadata = {
        "registry_version": 1,
        "registry_fingerprint": registry_fingerprint,
        "messages": entries,
    }
    metadata_path = Path(args.metadata_out)
    metadata_path.parent.mkdir(parents=True, exist_ok=True)
    metadata_path.write_text(json.dumps(metadata, indent=2, sort_keys=True) + "\n", encoding="utf-8")


if __name__ == "__main__":
    main()
