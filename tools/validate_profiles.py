#!/usr/bin/env python3

import argparse
import hashlib
import json
import re
from pathlib import Path

import jsonschema
import yaml


PROFILE_SCHEMA_ID = "xgc2.adapter-profile.v1"
PROFILE_DIGEST_ALGORITHM = "sha256-raw-bytes"
KINDS = {"stream_out", "stream_in", "request_response", "operation"}
INPUT_KINDS = {"stream_in", "request_response", "operation"}
ENDPOINT_PARAMETER = re.compile(r"\{([a-z][a-z0-9_]*)\}")


def parse_args():
    parser = argparse.ArgumentParser(description="Validate XGC2 adapter profiles")
    parser.add_argument("--registry", required=True)
    parser.add_argument("--profiles", required=True)
    parser.add_argument("--schema")
    parser.add_argument("--metadata-out")
    return parser.parse_args()


def raw_sha256(path):
    """Return the lowercase SHA-256 hex digest of the file's complete raw bytes."""
    return hashlib.sha256(path.read_bytes()).hexdigest()


def load_message_registry(path):
    document = yaml.safe_load(path.read_text(encoding="utf-8"))
    entries = {}
    for item in document.get("messages", []):
        message_id = int(item["id"])
        entries[message_id] = {
            "type": str(item["type"]),
            "version": int(item["version"]),
            "roles": {str(role) for role in item.get("roles", [])},
        }
    if not entries:
        raise ValueError("message registry is empty")
    return entries


def load_profile_schema(path):
    document = json.loads(path.read_text(encoding="utf-8"))
    jsonschema.Draft7Validator.check_schema(document)
    return document


def require_registered_message(messages, channel, field, profile_path):
    message_id = channel.get(field)
    if not isinstance(message_id, int) or message_id <= 0:
        raise ValueError(
            "{}: channel {} requires {}".format(profile_path, channel.get("id"), field)
        )
    if message_id not in messages:
        raise ValueError(
            "{}: channel {} references unregistered message ID {}".format(
                profile_path, channel.get("id"), message_id
            )
        )
    return messages[message_id]


def validate_message_role(profile_path, channel, message, direction):
    kind = channel["kind"]
    roles = message["roles"]
    if direction == "output" and not roles.intersection({"telemetry", "diagnostic", "response"}):
        raise ValueError(
            "{}: {} output message {} has incompatible roles {}".format(
                profile_path, channel["id"], message["type"], sorted(roles)
            )
        )
    if direction == "input" and not roles.intersection({"control", "request"}):
        raise ValueError(
            "{}: {} {} input message {} has incompatible roles {}".format(
                profile_path, channel["id"], kind, message["type"], sorted(roles)
            )
        )


def validate_profile_document(profile_path, document, schema, messages):
    validator = jsonschema.Draft7Validator(schema)
    errors = sorted(
        validator.iter_errors(document),
        key=lambda error: tuple(str(item) for item in error.absolute_path),
    )
    if errors:
        error = errors[0]
        location = ".".join(str(item) for item in error.absolute_path) or "<root>"
        raise ValueError("{}: schema validation failed at {}: {}".format(profile_path, location, error.message))

    if document["schema"] != PROFILE_SCHEMA_ID:
        raise ValueError("{}: unsupported profile schema {}".format(profile_path, document["schema"]))

    profile_id = document["profile_id"]
    version_match = re.search(r"\.v([1-9][0-9]*)$", profile_id)
    if not version_match or int(version_match.group(1)) != int(document["profile_version"]):
        raise ValueError("{}: profile_id suffix and profile_version disagree".format(profile_path))

    namespace_parameter = document["namespace_parameter"]
    namespace_definition = document["parameters"].get(namespace_parameter)
    if namespace_definition is None:
        raise ValueError(
            "{}: namespace_parameter {} is not declared".format(profile_path, namespace_parameter)
        )
    if namespace_definition["type"] != "ros_namespace" or not namespace_definition["required"]:
        raise ValueError(
            "{}: namespace_parameter {} must be a required ros_namespace".format(
                profile_path, namespace_parameter
            )
        )

    seen_channels = set()
    for channel in document["channels"]:
        channel_id = channel["id"]
        kind = channel["kind"]
        if channel_id in seen_channels:
            raise ValueError("{}: duplicate channel ID {}".format(profile_path, channel_id))
        if kind not in KINDS:
            raise ValueError("{}: channel {} has invalid kind {}".format(profile_path, channel_id, kind))

        if kind in INPUT_KINDS:
            input_message = require_registered_message(messages, channel, "input_message_id", profile_path)
            validate_message_role(profile_path, channel, input_message, "input")
        if kind in {"stream_out", "request_response"}:
            output_message = require_registered_message(messages, channel, "output_message_id", profile_path)
            validate_message_role(profile_path, channel, output_message, "output")

        for endpoint_group in ("inputs",):
            for endpoint in channel.get(endpoint_group, {}).values():
                validate_endpoint_parameters(profile_path, document, channel_id, endpoint)
        for endpoint_name in ("output", "service"):
            endpoint = channel.get(endpoint_name)
            if endpoint is not None:
                validate_endpoint_parameters(profile_path, document, channel_id, endpoint)

        seen_channels.add(channel_id)

    for channel in document["channels"]:
        for observed_channel in channel.get("observes", []):
            if observed_channel == channel["id"]:
                raise ValueError(
                    "{}: channel {} cannot observe itself".format(profile_path, channel["id"])
                )
            if observed_channel not in seen_channels:
                raise ValueError(
                    "{}: channel {} observes unknown channel {}".format(
                        profile_path, channel["id"], observed_channel
                    )
                )

    return {
        "profile_id": profile_id,
        "profile_version": int(document["profile_version"]),
        "native_protocol": document["native_protocol"],
        "robot_kind": document["robot_kind"],
        "channel_ids": sorted(seen_channels),
    }


def validate_endpoint_parameters(profile_path, document, channel_id, endpoint):
    parameters = document["parameters"]
    for parameter_name in ENDPOINT_PARAMETER.findall(endpoint["name"]):
        definition = parameters.get(parameter_name)
        if definition is None:
            raise ValueError(
                "{}: channel {} endpoint references undeclared parameter {}".format(
                    profile_path, channel_id, parameter_name
                )
            )
        if not definition.get("required"):
            raise ValueError(
                "{}: channel {} endpoint parameter {} must be required".format(
                    profile_path, channel_id, parameter_name
                )
            )
        if definition["type"] not in {"string", "ros_namespace"}:
            raise ValueError(
                "{}: channel {} endpoint parameter {} must be a string".format(
                    profile_path, channel_id, parameter_name
                )
            )


def validate_profiles(registry_path, profiles_root, schema_path):
    messages = load_message_registry(registry_path)
    schema = load_profile_schema(schema_path)
    profile_paths = sorted(profiles_root.glob("*/*.yaml"))
    if not profile_paths:
        raise ValueError("no adapter profiles found")

    entries = []
    seen_profiles = set()
    for profile_path in profile_paths:
        document = yaml.safe_load(profile_path.read_text(encoding="utf-8"))
        entry = validate_profile_document(profile_path, document, schema, messages)
        profile_id = entry["profile_id"]
        if profile_id in seen_profiles:
            raise ValueError("duplicate profile ID: {}".format(profile_id))
        seen_profiles.add(profile_id)
        entry["digest"] = raw_sha256(profile_path)
        entry["digest_algorithm"] = PROFILE_DIGEST_ALGORITHM
        entry["file"] = profile_path.relative_to(profiles_root).as_posix()
        entries.append(entry)

    entries.sort(key=lambda item: item["profile_id"])
    catalog_hash = hashlib.sha256()
    for entry in entries:
        catalog_hash.update(entry["profile_id"].encode("utf-8"))
        catalog_hash.update(b"\0")
        catalog_hash.update(entry["digest"].encode("ascii"))
        catalog_hash.update(b"\n")

    return {
        "schema": "xgc2.adapter-profile-registry.v1",
        "profile_schema": PROFILE_SCHEMA_ID,
        "profile_schema_digest": raw_sha256(schema_path),
        "digest_algorithm": PROFILE_DIGEST_ALGORITHM,
        "catalog_digest": catalog_hash.hexdigest(),
        "profiles": entries,
    }


def main():
    args = parse_args()
    profiles_root = Path(args.profiles)
    schema_path = (
        Path(args.schema)
        if args.schema
        else profiles_root / "schema" / "adapter-profile-v1.schema.json"
    )
    metadata = validate_profiles(Path(args.registry), profiles_root, schema_path)

    if args.metadata_out:
        output_path = Path(args.metadata_out)
        output_path.parent.mkdir(parents=True, exist_ok=True)
        output_path.write_text(json.dumps(metadata, indent=2, sort_keys=True) + "\n", encoding="utf-8")

    channel_count = sum(len(item["channel_ids"]) for item in metadata["profiles"])
    print(
        "validated {} profiles and {} channels; catalog_digest={}".format(
            len(metadata["profiles"]), channel_count, metadata["catalog_digest"]
        )
    )


if __name__ == "__main__":
    main()
