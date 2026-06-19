#!/usr/bin/env python3
"""Copy all image digests from staging overlay to production overlay."""
from __future__ import annotations

import re
from pathlib import Path

STAGING = Path("k8s/overlays/staging/kustomization.yaml")
PROD = Path("k8s/overlays/production/kustomization.yaml")


def extract_images(text: str) -> dict[str, str]:
    blocks = re.findall(
        r"name:\s+(ghcr\.io/[^\n]+)\n(?:\s+newName:[^\n]+\n)?\s+digest:\s+(sha256:[a-f0-9]+)",
        text,
    )
    return dict(blocks)


def patch_prod(prod_text: str, digests: dict[str, str]) -> str:
    for image, digest in digests.items():
        pattern = (
            rf"(name:\s+{re.escape(image)}\s*\n"
            rf"(?:\s+newName:[^\n]+\n)?"
            rf"\s+digest:\s+)sha256:[a-f0-9]+"
        )
        prod_text, n = re.subn(pattern, r"\1" + digest, prod_text, count=1)
        if n != 1:
            raise SystemExit(f"failed to patch {image}")
    return prod_text


def main() -> None:
    staging = STAGING.read_text()
    digests = extract_images(staging)
    if not digests:
        raise SystemExit("no digests in staging overlay")
    for d in digests.values():
        if d == "sha256:0000000000000000000000000000000000000000000000000000000000000000":
            raise SystemExit("staging has placeholder digest")
    prod_text = patch_prod(PROD.read_text(), digests)
    PROD.write_text(prod_text)


if __name__ == "__main__":
    main()
