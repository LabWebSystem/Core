#!/usr/bin/env python3
"""本番Composeを正規化し、隔離QA用イメージ参照を設定する。"""

import argparse
import json
import os
from pathlib import Path
import subprocess
import sys


ROOT = Path(__file__).resolve().parent.parent
COMPOSE = ROOT / "infrastructure" / "compose.yaml"


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--output", required=True, type=Path)
    parser.add_argument("--project", required=True)
    parser.add_argument("--backend-image", required=True)
    parser.add_argument("--avahi-image", required=True)
    parser.add_argument("--base-domain", required=True)
    parser.add_argument("--installation-id", required=True)
    args = parser.parse_args()

    environment = os.environ.copy()
    environment["LWS_BASE_DOMAIN"] = args.base_domain
    environment["LWS_INSTALLATION_ID"] = args.installation_id
    result = subprocess.run(
        ["docker", "compose", "--project-name", args.project, "--file", str(COMPOSE), "config", "--format", "json"],
        check=False,
        capture_output=True,
        text=True,
        env=environment,
    )
    if result.returncode != 0:
        sys.stderr.write(result.stderr)
        return result.returncode

    config = json.loads(result.stdout)
    services = config.get("services", {})
    backend = services.get("backend", {})
    build = backend.get("build", {})
    context = Path(build.get("context", "")).resolve()
    dockerfile = Path(build.get("dockerfile", ""))
    if context != ROOT or (context / dockerfile).resolve() != ROOT / "backend" / "Dockerfile":
        print("BackendのDocker build contextまたはDockerfileが想定と異なります", file=sys.stderr)
        return 1

    backend["image"] = args.backend_image
    backend["pull_policy"] = "never"
    backend_environment = backend.setdefault("environment", {})
    backend_environment["LWS_BASE_DOMAIN"] = args.base_domain
    backend_environment["LWS_INSTALLATION_ID"] = args.installation_id
    avahi = services.get("avahi")
    if avahi is None:
        print("ComposeにAvahi serviceがありません", file=sys.stderr)
        return 1
    avahi["image"] = args.avahi_image
    avahi["pull_policy"] = "never"

    networks = config.get("networks", {})
    internal = networks.get("lws-internal")
    if internal is None:
        print("ComposeにLWS内部networkがありません", file=sys.stderr)
        return 1
    internal["name"] = f"{args.project}-internal"

    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.output.write_text(json.dumps(config, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
