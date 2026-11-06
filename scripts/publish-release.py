#!/usr/bin/env python3
"""Publish an idempotent Gitea release only after public revision verification."""
import json
import os
import subprocess
import urllib.error
import urllib.request

from verify_release import verify


def main():
    revision = subprocess.check_output(["git", "rev-parse", "HEAD"], text=True).strip()
    verify(revision)
    tag = "main-" + revision[:12]
    endpoint = "https://git.subcult.tv/api/v1/repos/subculture-collective/invisible-museum/releases"
    headers = {
        "Authorization": "token " + os.environ["GITEA_TOKEN"],
        "Content-Type": "application/json",
    }
    try:
        with urllib.request.urlopen(urllib.request.Request(endpoint + "/tags/" + tag, headers=headers), timeout=30) as response:
            release = json.load(response)
            assert release["target_commitish"] == revision
            print("Release already published:", release["html_url"])
            return
    except urllib.error.HTTPError as error:
        if error.code != 404:
            raise RuntimeError(f"Release lookup failed: HTTP {error.code}") from None
    data = {
        "tag_name": tag,
        "target_commitish": revision,
        "name": "Byl " + tag,
        "body": f"Automatic release of `{revision}` to https://byl.subcult.tv.\n\n"
        "Backend, web, Android and iOS checks passed before image build and deployment. "
        "Release images passed disposable-database/static smoke checks. "
        "Public revision, web, API, seeded tours and disabled development sign-in passed after activation. "
        "Database schema changes require operator admission. Native app binaries are not published by this workflow.",
        "draft": False,
        "prerelease": True,
    }
    request = urllib.request.Request(endpoint, headers=headers, data=json.dumps(data).encode(), method="POST")
    try:
        with urllib.request.urlopen(request, timeout=30) as response:
            print("Published release:", json.load(response)["html_url"])
    except urllib.error.HTTPError as error:
        raise RuntimeError(f"Release publication failed: HTTP {error.code}") from None


if __name__ == "__main__":
    main()
