#!/usr/bin/env bash
# Test the committed CI checkout with the same checksum-verified Gradle release.
# Image source: gradle/docker-gradle fba2d36, jdk21-noble/Dockerfile.
set -euo pipefail
repo_root=$(git rev-parse --show-toplevel)
cd "$repo_root"
android_image=gradle@sha256:21bd311ed01360c189b8870c6b6e988199ff10f72d445d02fb39d3cff9da91d7
wrapper=android/gradle/wrapper/gradle-wrapper.properties
[[ $(sed -n 's/^distributionSha256Sum=//p' "$wrapper") == bd71102213493060956ec229d946beee57158dbd89d0e62b91bca0fa2c5f3531 ]] || {
  echo 'Update the pinned CI image when the Gradle wrapper distribution changes.' >&2
  exit 1
}
container_id=""
# shellcheck disable=SC2329 # Invoked indirectly by the EXIT trap.
cleanup() {
  if [[ -n "$container_id" ]]; then docker rm -fv "$container_id" >/dev/null; fi
}
trap 'cleanup' EXIT
printf 'Testing committed Android core at %s\n' "$(git rev-parse HEAD)"
container_id=$(docker create --interactive --init --cpus 2 --memory 4g \
  --label subcult.task=byl-android-core --entrypoint bash "$android_image" -euc '
    test "$GRADLE_VERSION" = 8.14.3
    mkdir /tmp/byl-source
    tar -xf - -C /tmp/byl-source
    cd /tmp/byl-source/android
    exec gradle --no-daemon --console=plain :core:test
  ')
# Stream source instead of host bind mounts: Actions runs inside a job container.
git archive --format=tar HEAD android backend/api/fixtures | docker start --attach --interactive "$container_id"
container_exit=$(docker inspect --format '{{.State.ExitCode}}' "$container_id")
exit "$container_exit"
