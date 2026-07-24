#!/usr/bin/env python3
"""Run the critical handler coverage gate with explicit non-regression floors."""

from __future__ import annotations

import argparse
import re
import subprocess
import sys
import tempfile
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
FUNCTION_FLOORS = {
    "PortfolioAssistantChatStreamHandler": 90.0,
    "decodeJSONWithLimit": 90.0,
    "SessionLogoutHandler": 70.0,
    "AdminReplyChatSessionHandler": 50.0,
}


def run(*command: str) -> str:
    result = subprocess.run(command, cwd=ROOT, text=True, capture_output=True, check=False)
    if result.returncode != 0:
        sys.stderr.write(result.stdout)
        sys.stderr.write(result.stderr)
        raise RuntimeError(f"command failed: {' '.join(command[:3])}")
    return result.stdout


def parse_coverage(output: str) -> tuple[float, dict[str, float]]:
    functions: dict[str, float] = {}
    total: float | None = None
    for line in output.splitlines():
        percent_match = re.search(r"([0-9]+(?:\.[0-9]+)?)%$", line)
        if not percent_match:
            continue
        value = float(percent_match.group(1))
        if line.startswith("total:"):
            total = value
            continue
        name_match = re.search(r":\d+:\s+([^\s]+)\s+[0-9.]+%$", line)
        if name_match:
            functions[name_match.group(1)] = value
    if total is None:
        raise ValueError("unable to parse total handler coverage")
    return total, functions


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--minimum-total", type=float, default=45.0)
    args = parser.parse_args()
    try:
        with tempfile.NamedTemporaryFile(suffix=".out") as profile:
            run("go", "test", "./internal/handler", f"-coverprofile={profile.name}", "-count=1")
            report = run("go", "tool", "cover", f"-func={profile.name}")
        total, functions = parse_coverage(report)
        failures: list[str] = []
        if total < args.minimum_total:
            failures.append(f"total {total:.1f}% < {args.minimum_total:.1f}%")
        for name, minimum in FUNCTION_FLOORS.items():
            actual = functions.get(name)
            if actual is None:
                failures.append(f"critical function missing: {name}")
            elif actual < minimum:
                failures.append(f"{name} {actual:.1f}% < {minimum:.1f}%")
        if failures:
            print("handler coverage gate failed: " + "; ".join(failures), file=sys.stderr)
            return 1
        print(f"handler coverage gate passed: total={total:.1f}% critical_functions={len(FUNCTION_FLOORS)}")
        return 0
    except (OSError, RuntimeError, ValueError) as error:
        print(f"handler coverage gate failed: {error}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
