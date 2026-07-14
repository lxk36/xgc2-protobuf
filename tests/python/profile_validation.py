#!/usr/bin/env python3

import copy
import hashlib
import importlib.util
import re
import tempfile
from pathlib import Path

import yaml


ROOT = Path(__file__).resolve().parents[2]
SPEC = importlib.util.spec_from_file_location(
    "validate_profiles", ROOT / "tools" / "validate_profiles.py"
)
validate_profiles = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(validate_profiles)


def expect_failure(callback, expected):
    try:
        callback()
    except ValueError as error:
        assert expected in str(error), str(error)
        return
    raise AssertionError("expected validation failure containing {!r}".format(expected))


def main():
    profiles_root = ROOT / "profiles"
    registry_path = ROOT / "registry" / "messages.yaml"
    schema_path = profiles_root / "schema" / "adapter-profile-v1.schema.json"
    metadata = validate_profiles.validate_profiles(registry_path, profiles_root, schema_path)

    assert metadata["schema"] == "xgc2.adapter-profile-registry.v1"
    assert metadata["digest_algorithm"] == "sha256-raw-bytes"
    assert re.fullmatch(r"[0-9a-f]{64}", metadata["catalog_digest"])
    assert re.fullmatch(r"[0-9a-f]{64}", metadata["profile_schema_digest"])
    entries = {item["profile_id"]: item for item in metadata["profiles"]}
    assert set(entries) == {"px4.multirotor.ros1.v1", "scout-mini.ros1.v1"}

    for entry in entries.values():
        profile_path = profiles_root / entry["file"]
        assert entry["digest"] == hashlib.sha256(profile_path.read_bytes()).hexdigest()
        assert re.fullmatch(r"[0-9a-f]{64}", entry["digest"])
        assert not any(channel.startswith("control.") for channel in entry["channel_ids"])
        assert "diagnostic.channel-health" in entry["channel_ids"]

    px4 = entries["px4.multirotor.ros1.v1"]
    assert {"operation.arm", "operation.mode", "operation.autopilot-reboot"}.issubset(
        set(px4["channel_ids"])
    )
    scout = entries["scout-mini.ros1.v1"]
    assert not any(channel.startswith("operation.") for channel in scout["channel_ids"])

    schema = validate_profiles.load_profile_schema(schema_path)
    messages = validate_profiles.load_message_registry(registry_path)
    source_path = profiles_root / "ros1" / "scout-mini-ros1-v1.yaml"
    source = yaml.safe_load(source_path.read_text(encoding="utf-8"))

    missing_namespace = copy.deepcopy(source)
    del missing_namespace["parameters"]["namespace"]
    expect_failure(
        lambda: validate_profiles.validate_profile_document(
            source_path, missing_namespace, schema, messages
        ),
        "namespace_parameter namespace is not declared",
    )

    unknown_message = copy.deepcopy(source)
    unknown_message["channels"][0]["output_message_id"] = 999999
    expect_failure(
        lambda: validate_profiles.validate_profile_document(
            source_path, unknown_message, schema, messages
        ),
        "unregistered message ID 999999",
    )

    absolute_endpoint = copy.deepcopy(source)
    absolute_endpoint["channels"][0]["inputs"]["odometry"]["name"] = "/odom"
    expect_failure(
        lambda: validate_profiles.validate_profile_document(
            source_path, absolute_endpoint, schema, messages
        ),
        "schema validation failed",
    )

    with tempfile.TemporaryDirectory() as directory:
        probe = Path(directory) / "profile.yaml"
        probe.write_bytes(b"profile: bytes\n")
        first = validate_profiles.raw_sha256(probe)
        probe.write_bytes(b"profile: bytes\n\n")
        second = validate_profiles.raw_sha256(probe)
        assert first == hashlib.sha256(b"profile: bytes\n").hexdigest()
        assert first != second

    print("profile validation and raw-byte digest tests passed")


if __name__ == "__main__":
    main()
