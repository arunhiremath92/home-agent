#!/usr/bin/env python3
"""Add location memories through the versioned agent-server REST API."""

import argparse
import json
import random
import urllib.error
import urllib.request


DEFAULT_LATITUDE = 40.7580
DEFAULT_LONGITUDE = -73.9855


def parse_args(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--store-name", required=True, help="name of the store")
    parser.add_argument("--store-type", required=True, help="store type, e.g. restaurant or grocery")
    parser.add_argument(
        "--message",
        action="append",
        required=True,
        help="memory to save; repeat this option to add multiple memories",
    )
    parser.add_argument("--latitude", type=float, help="store latitude; provide together with --longitude")
    parser.add_argument("--longitude", type=float, help="store longitude; provide together with --latitude")
    parser.add_argument(
        "--endpoint",
        default="http://localhost:8080/api/v1/location-memories",
        help="agent-server location memories endpoint (default: %(default)s)",
    )
    args = parser.parse_args(argv)

    return parser, args


def main(argv=None):
    parser, args = parse_args(argv)
    if (args.latitude is None) != (args.longitude is None):
        parser.error("--latitude and --longitude must be provided together")
    if args.latitude is None:
        latitude = DEFAULT_LATITUDE + random.uniform(-0.02, 0.02)
        longitude = DEFAULT_LONGITUDE + random.uniform(-0.02, 0.02)
        print(f"Using generated fictional coordinates: {latitude:.6f}, {longitude:.6f}")
    else:
        latitude = args.latitude
        longitude = args.longitude
    if not -90 <= latitude <= 90:
        parser.error("--latitude must be between -90 and 90")
    if not -180 <= longitude <= 180:
        parser.error("--longitude must be between -180 and 180")

    for index, memory in enumerate(args.message, start=1):
        payload = {
            "store_type": args.store_type,
            "store_name": args.store_name,
            "latitude": latitude,
            "longitude": longitude,
            "message": memory,
        }
        request = urllib.request.Request(
            args.endpoint,
            data=json.dumps(payload).encode("utf-8"),
            headers={"Content-Type": "application/json"},
            method="POST",
        )
        try:
            with urllib.request.urlopen(request, timeout=10) as response:
                response_body = response.read().decode("utf-8").strip()
                if response.status < 200 or response.status >= 300:
                    raise RuntimeError(
                        f"request {index} failed with HTTP {response.status}: "
                        f"{response_body}"
                    )
                print(f"Added memory {index}/{len(args.message)}: {memory}")
        except urllib.error.HTTPError as error:
            body = error.read().decode("utf-8", errors="replace").strip()
            error.close()
            raise SystemExit(
                f"Request {index} failed with HTTP {error.code}: {body}"
            ) from error
        except urllib.error.URLError as error:
            raise SystemExit(
                f"Could not reach {args.endpoint}: {error.reason}"
            ) from error


if __name__ == "__main__":
    main()
