#!/usr/bin/env python3
"""Coupler MCP server — stdio JSON-RPC (Phase 0).

Exposes tool catalog and dry-run execute. Optionally proxies to coupler-gateway.
"""
from __future__ import annotations

import json
import os
import sys
import urllib.error
import urllib.request
from pathlib import Path
from typing import Any

GATEWAY_URL = os.environ.get("COUPLER_GATEWAY_URL", "http://localhost:8787").rstrip("/")
REPO_ROOT = Path(__file__).resolve().parents[2]
CONNECTORS_DIR = Path(os.environ.get("COUPLER_CONNECTORS_DIR", REPO_ROOT / "connectors"))


def load_tools() -> list[dict[str, Any]]:
    try:
        with urllib.request.urlopen(f"{GATEWAY_URL}/v1/tools", timeout=3) as resp:
            data = json.loads(resp.read().decode())
            return data.get("tools", [])
    except (urllib.error.URLError, TimeoutError, json.JSONDecodeError):
        return _load_tools_from_manifests()


def _load_tools_from_manifests() -> list[dict[str, Any]]:
    tools: list[dict[str, Any]] = []
    if not CONNECTORS_DIR.is_dir():
        return tools
    try:
        import yaml  # optional; fallback to minimal parse if missing
    except ImportError:
        yaml = None  # type: ignore

    for manifest in sorted(CONNECTORS_DIR.glob("*/manifest.yaml")):
        text = manifest.read_text()
        if yaml:
            doc = yaml.safe_load(text) or {}
            connector = doc.get("connector", manifest.parent.name)
            for t in doc.get("tools", []):
                tools.append({**t, "connector": connector})
        else:
            # minimal: only names from lines starting with "  - name:"
            connector = manifest.parent.name
            for line in text.splitlines():
                if line.strip().startswith("- name:"):
                    name = line.split(":", 1)[1].strip()
                    tools.append({"name": name, "description": "", "connector": connector})
    return sorted(tools, key=lambda t: t.get("name", ""))


def mcp_tools() -> list[dict[str, Any]]:
    out = []
    for t in load_tools():
        schema = t.get("parameters") or {"type": "object", "properties": {}}
        out.append({
            "name": t["name"],
            "description": t.get("description", ""),
            "inputSchema": schema,
        })
    out.append({
        "name": "coupler_search_tools",
        "description": "Search the Coupler tool catalog by keyword",
        "inputSchema": {
            "type": "object",
            "required": ["query"],
            "properties": {"query": {"type": "string"}},
        },
    })
    return out


def gateway_execute(tool: str, arguments: dict[str, Any], dry_run: bool = True) -> dict[str, Any]:
    body = json.dumps({"tool": tool, "arguments": arguments, "dry_run": dry_run}).encode()
    req = urllib.request.Request(
        f"{GATEWAY_URL}/v1/tools/execute",
        data=body,
        headers={"Content-Type": "application/json"},
        method="POST",
    )
    with urllib.request.urlopen(req, timeout=10) as resp:
        return json.loads(resp.read().decode())


def handle_call(name: str, arguments: dict[str, Any]) -> dict[str, Any]:
    if name == "coupler_search_tools":
        q = arguments.get("query", "")
        try:
            with urllib.request.urlopen(
                f"{GATEWAY_URL}/v1/tools/search?q={urllib.parse.quote(q)}", timeout=3
            ) as resp:
                return json.loads(resp.read().decode())
        except Exception:
            tools = load_tools()
            ql = q.lower()
            matched = [
                t for t in tools
                if ql in t.get("name", "").lower() or ql in t.get("description", "").lower()
            ]
            return {"query": q, "tools": matched, "count": len(matched)}

    dry_run = arguments.pop("dry_run", True)
    try:
        return gateway_execute(name, arguments, dry_run=dry_run)
    except urllib.error.HTTPError as e:
        payload = e.read().decode()
        try:
            return json.loads(payload)
        except json.JSONDecodeError:
            return {"error": payload, "status": e.code}
    except Exception as e:
        return {
            "dry_run": True,
            "tool": name,
            "arguments": arguments,
            "message": f"Gateway unavailable ({e}); local dry-run only",
        }


# --- stdio MCP loop (minimal JSON-RPC 2.0) ---

import urllib.parse  # noqa: E402


def send(msg: dict[str, Any]) -> None:
    sys.stdout.write(json.dumps(msg) + "\n")
    sys.stdout.flush()


def run() -> None:
    for line in sys.stdin:
        line = line.strip()
        if not line:
            continue
        try:
            req = json.loads(line)
        except json.JSONDecodeError:
            continue

        rid = req.get("id")
        method = req.get("method", "")
        params = req.get("params") or {}

        if method == "initialize":
            send({
                "jsonrpc": "2.0",
                "id": rid,
                "result": {
                    "protocolVersion": "2024-11-05",
                    "capabilities": {"tools": {}},
                    "serverInfo": {"name": "coupler-mcp", "version": "0.1.0"},
                },
            })
        elif method == "notifications/initialized":
            pass
        elif method == "tools/list":
            send({
                "jsonrpc": "2.0",
                "id": rid,
                "result": {"tools": mcp_tools()},
            })
        elif method == "tools/call":
            name = params.get("name", "")
            args = params.get("arguments") or {}
            result = handle_call(name, args)
            send({
                "jsonrpc": "2.0",
                "id": rid,
                "result": {
                    "content": [{"type": "text", "text": json.dumps(result, indent=2)}],
                    "isError": "error" in result and not result.get("dry_run"),
                },
            })
        elif method == "ping":
            send({"jsonrpc": "2.0", "id": rid, "result": {}})
        else:
            if rid is not None:
                send({
                    "jsonrpc": "2.0",
                    "id": rid,
                    "error": {"code": -32601, "message": f"Method not found: {method}"},
                })


if __name__ == "__main__":
    run()
