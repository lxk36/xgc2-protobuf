#!/usr/bin/env python3

import argparse
from pathlib import Path

import yaml


KINDS = {"stream_out", "stream_in", "request_response"}


def parse_args():
    parser = argparse.ArgumentParser(description="Validate XGC2 adapter profiles")
    parser.add_argument("--registry", required=True)
    parser.add_argument("--profiles", required=True)
    return parser.parse_args()


def require_message(message_ids, channel, field, profile_path):
    value = channel.get(field)
    if not isinstance(value, int) or value <= 0:
        raise ValueError("{}: channel {} requires {}".format(profile_path, channel.get("id"), field))
    if value not in message_ids:
        raise ValueError(
            "{}: channel {} references unregistered message ID {}".format(
                profile_path, channel.get("id"), value
            )
        )


def validate_profile(profile_path, message_ids):
    document = yaml.safe_load(profile_path.read_text(encoding="utf-8"))
    if not document.get("profile") or not document.get("native_protocol"):
        raise ValueError("{}: profile and native_protocol are required".format(profile_path))

    seen_channels = set()
    for channel in document.get("channels", []):
        channel_id = channel.get("id")
        kind = channel.get("kind")
        if not channel_id or channel_id in seen_channels:
            raise ValueError("{}: channel IDs must be present and unique".format(profile_path))
        if kind not in KINDS:
            raise ValueError("{}: channel {} has invalid kind {}".format(profile_path, channel_id, kind))
        if not channel.get("processor"):
            raise ValueError("{}: channel {} requires a processor".format(profile_path, channel_id))

        if kind == "stream_out":
            require_message(message_ids, channel, "output_message_id", profile_path)
            if not channel.get("inputs"):
                raise ValueError("{}: stream_out channel {} requires inputs".format(profile_path, channel_id))
        elif kind == "stream_in":
            require_message(message_ids, channel, "input_message_id", profile_path)
            if not channel.get("output"):
                raise ValueError("{}: stream_in channel {} requires output".format(profile_path, channel_id))
        else:
            require_message(message_ids, channel, "input_message_id", profile_path)
            require_message(message_ids, channel, "output_message_id", profile_path)
            if not channel.get("service"):
                raise ValueError(
                    "{}: request_response channel {} requires service".format(profile_path, channel_id)
                )
        seen_channels.add(channel_id)

    if not seen_channels:
        raise ValueError("{}: at least one channel is required".format(profile_path))
    return document["profile"], len(seen_channels)


def main():
    args = parse_args()
    registry = yaml.safe_load(Path(args.registry).read_text(encoding="utf-8"))
    message_ids = {int(item["id"]) for item in registry.get("messages", [])}
    if not message_ids:
        raise ValueError("message registry is empty")

    profile_paths = sorted(Path(args.profiles).rglob("*.yaml"))
    if not profile_paths:
        raise ValueError("no adapter profiles found")

    seen_profiles = set()
    channel_count = 0
    for profile_path in profile_paths:
        profile_id, count = validate_profile(profile_path, message_ids)
        if profile_id in seen_profiles:
            raise ValueError("duplicate profile ID: {}".format(profile_id))
        seen_profiles.add(profile_id)
        channel_count += count

    print("validated {} profiles and {} channels".format(len(seen_profiles), channel_count))


if __name__ == "__main__":
    main()
