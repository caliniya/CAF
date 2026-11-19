"""Select supported Wails targets for CI and release tags."""

import argparse
import json
import os
from pathlib import Path
import re


TARGETS = (
    {"platform": "windows", "goos": "windows", "arch": "amd64", "runner": "windows-2025"},
    {"platform": "windows", "goos": "windows", "arch": "arm64", "runner": "windows-2025"},
    {"platform": "linux", "goos": "linux", "arch": "amd64", "runner": "ubuntu-24.04"},
    {"platform": "linux", "goos": "linux", "arch": "arm64", "runner": "ubuntu-24.04-arm"},
    {"platform": "mac", "goos": "darwin", "arch": "amd64", "runner": "macos-15-intel"},
    {"platform": "mac", "goos": "darwin", "arch": "arm64", "runner": "macos-15"},
    {"platform": "windows", "goos": "windows", "arch": "386", "runner": "windows-2025"},
    {"platform": "linux", "goos": "linux", "arch": "arm", "runner": "ubuntu-24.04-arm"},
)

ALL_TAG = re.compile(r"v[0-9]+\.[0-9]+(?P<all>-?all)?")
PLATFORM_TAG = re.compile(r"v[0-9]+\.[0-9]+\.[0-9]+-(?P<platform>windows|linux|mac)")


def select_targets(mode: str, tag: str = "") -> list[dict[str, str]]:
    if mode == "ci":
        # One native build per OS; release tags cover the remaining architectures.
        return [target for target in TARGETS if (target["platform"], target["arch"]) in {
            ("windows", "amd64"), ("linux", "amd64"), ("mac", "arm64"),
        }]

    if mode == "all":
        match = ALL_TAG.fullmatch(tag)
        if match is None:
            return []
        return [target for target in TARGETS if match["all"] or target["arch"] in {"amd64", "arm64"}]

    if mode == "platform":
        match = PLATFORM_TAG.fullmatch(tag)
        if match is None:
            return []
        return [target for target in TARGETS
                if target["platform"] == match["platform"] and target["arch"] in {"amd64", "arm64"}]

    raise ValueError(f"Unknown build mode: {mode}")


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("mode", choices=("ci", "all", "platform"))
    args = parser.parse_args()
    tag = os.environ.get("RELEASE_TAG", "")
    targets = select_targets(args.mode, tag)
    outputs = {
        "enabled": "true" if targets else "false",
        "matrix": json.dumps({"include": targets}, separators=(",", ":")),
    }
    with Path(os.environ["GITHUB_OUTPUT"]).open("a", encoding="utf-8") as output:
        for name, value in outputs.items():
            output.write(f"{name}={value}\n")
    print(f"Selected {len(targets)} targets for {args.mode}: {tag or 'CI'}")


if __name__ == "__main__":
    main()
