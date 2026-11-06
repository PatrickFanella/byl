#!/usr/bin/env bash
set -euo pipefail
revision=$(git rev-parse HEAD)
[[ $revision =~ ^[0-9a-f]{40}$ ]]
deploy=${DEPLOY:-false}
scratch=$(mktemp -d)
cleanup() { rm -rf "$scratch"; }
trap cleanup EXIT

if [[ $deploy == true ]]; then
  : "${HOMELAB_DEPLOY_KEY:?missing app-specific deploy key}"
  : "${HOMELAB_KNOWN_HOSTS:?missing pinned host key}"
  umask 077
  printf '%s\n' "$HOMELAB_DEPLOY_KEY" > "$scratch/key"
  printf '%s\n' "$HOMELAB_KNOWN_HOSTS" > "$scratch/known_hosts"
  cat > "$scratch/ssh_config" <<EOF
Host byl-deploy
  HostName 10.0.0.56
  User onnwee
  IdentityFile $scratch/key
  UserKnownHostsFile $scratch/known_hosts
  StrictHostKeyChecking yes
  IdentitiesOnly yes
  BatchMode yes
  ConnectTimeout 15
  ControlMaster no
  ControlPath none
EOF
  unset HOMELAB_DEPLOY_KEY HOMELAB_KNOWN_HOSTS
fi

for service in api web; do
  image="fleet-deploy/byl/$service:$revision"
  if [[ $deploy == true ]]; then
    ssh -F "$scratch/ssh_config" byl-deploy "artifact $revision $service" > "$scratch/artifact.json"
    if python3 -c 'import json,sys; sys.exit(0 if json.load(open(sys.argv[1]))["available"] else 1)' "$scratch/artifact.json"; then
      echo "Reusing accepted $service image for $revision"
      continue
    fi
  fi
  if [[ $service == api ]]; then
    docker build --build-arg "REVISION=$revision" -t "$image" backend
  else
    docker build --build-arg "REVISION=$revision" \
      --build-arg "VITE_GOOGLE_CLIENT_ID=${VITE_GOOGLE_CLIENT_ID:-}" \
      -f web/Dockerfile -t "$image" .
  fi
  bash scripts/smoke-release.sh "$service" "$image" "$revision"
  if [[ $deploy == true ]]; then
    docker save "$image" | ssh -F "$scratch/ssh_config" byl-deploy "receive $revision $service"
  fi
done

if [[ $deploy == true ]]; then
  ssh -F "$scratch/ssh_config" byl-deploy "deploy $revision"
  python3 scripts/verify_release.py "$revision"
fi
