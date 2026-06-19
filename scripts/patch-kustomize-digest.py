#!/usr/bin/env python3
"""Patch a single image digest in a kustomization.yaml by image name."""
from __future__ import annotations

import argparse
import re
import sys
from pathlib import Path


def patch_digest(text: str, image: str, digest: str) -> tuple[str, int]:
    if not digest.startswith("sha256:"):
        raise ValueError(f"invalid digest: {digest!r}")
    pattern = (
        rf"(name:\s+{re.escape(image)}\s*\n"
        rf"(?:\s+newName:[^\n]+\n)?"
        rf"\s+digest:\s+)sha256:[a-f0-9]+"
    )
    return re.subn(pattern, r"\1" + digest, text, count=1)


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("file", type=Path)
    parser.add_argument("image", help="Full image name, e.g. ghcr.io/madfam-org/coupler-gateway")
    parser.add_argument("digest")
    args = parser.parse_args()
    text = args.file.read_text()
    updated, n = patch_digest(text, args.image, args.digest)
    if n != 1:
        print(f"digest not patched for {args.image} (matches={n})", file=sys.stderr)
        return 1
    args.file.write_text(updated)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
