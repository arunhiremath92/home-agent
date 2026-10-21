#!/usr/bin/env python3
"""Add one task through the versioned agent-server REST API."""

import argparse
import json
import urllib.error
import urllib.request


def parse_args(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--title", required=True, help="task title")
    parser.add_argument("--description", default="", help="optional task description")
    parser.add_argument(
        "--due-at",
        help="optional due date/time in RFC3339 format, e.g. 2026-10-17T09:30:00Z",
    )
    parser.add_argument(
        "--endpoint",
        default="http://localhost:8080/api/v1/tasks",
        help="agent-server task endpoint (default: %(default)s)",
    )
    return parser.parse_args(argv)


def main(argv=None):
    args = parse_args(argv)
    payload = {
        "title": args.title,
        "description": args.description,
    }
    if args.due_at:
        payload["due_at"] = args.due_at

    request = urllib.request.Request(
        args.endpoint,
        data=json.dumps(payload).encode("utf-8"),
        headers={"Content-Type": "application/json"},
        method="POST",
    )
    try:
        with urllib.request.urlopen(request, timeout=10) as response:
            response_body = response.read().decode("utf-8", errors="replace").strip()
            if response.status < 200 or response.status >= 300:
                raise SystemExit(
                    f"Task creation failed with HTTP {response.status}: {response_body}"
                )
            print(f"Created task: {response_body}")
    except urllib.error.HTTPError as error:
        body = error.read().decode("utf-8", errors="replace").strip()
        error.close()
        raise SystemExit(
            f"Task creation failed with HTTP {error.code}: {body}"
        ) from error
    except urllib.error.URLError as error:
        raise SystemExit(
            f"Could not reach {args.endpoint}: {error.reason}"
        ) from error


if __name__ == "__main__":
    main()
