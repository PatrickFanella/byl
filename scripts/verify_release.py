#!/usr/bin/env python3
"""Read-only conformance checks against the public deployment."""
import json
import sys
import urllib.error
import urllib.request


def get(path):
    request = urllib.request.Request(
        "https://byl.subcult.tv" + path,
        headers={"Cache-Control": "no-cache", "User-Agent": "Byl-release-check/1"},
    )
    with urllib.request.urlopen(request, timeout=20) as response:
        assert response.status == 200, path
        return response.read()


def verify(revision):
    assert json.loads(get("/release.json")) == {"revision": revision}
    assert b"_app/immutable" in get("/")
    assert b"_app/immutable" in get("/admin")
    get("/healthz")
    cities = json.loads(get("/api/v1/cities"))["items"]
    assert any(city["slug"] == "downers-grove" for city in cities)
    tours = json.loads(get("/api/v1/cities/downers-grove/tours"))["items"]
    assert tours, "Expected seeded tours"
    get("/brand/favicon.svg")
    request = urllib.request.Request(
        "https://byl.subcult.tv/api/v1/auth/dev",
        data=b'{"email":"release-check@example.invalid"}',
        headers={"Content-Type": "application/json", "User-Agent": "Byl-release-check/1"},
    )
    try:
        urllib.request.urlopen(request, timeout=20)
    except urllib.error.HTTPError as error:
        assert error.code == 404, "Development sign-in must be disabled"
    else:
        raise AssertionError("Development sign-in must be disabled")
    print(f"PASS public release {revision}: web/admin, API, seeded tours, favicon, auth boundary")


if __name__ == "__main__":
    verify(sys.argv[1])
