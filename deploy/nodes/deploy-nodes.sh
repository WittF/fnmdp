#!/usr/bin/env bash
# Batch-deploy the fnmdp collector to a fleet of SSH hosts.
#
# Usage:
#   NODE_TOKEN=<token> SSH_PASS=<password> ./deploy-nodes.sh hosts.txt [N]
#
#   hosts.txt — one host per line: "<ip> <node-id>" (whitespace separated).
#               The node-id becomes NODE_ID / part of NODE_NAME. Lines starting
#               with # are skipped. This file is NOT committed (contains the IPs).
#   N         — optional: only deploy the first N hosts (staged rollout).
#
# Env:
#   NODE_TOKEN  (required) must match the master's node-token secret.
#   SSH_PASS    (required) SSH password (uses sshpass).
#   SSH_USER    (default: root)
#   REGION      (default: unknown)
#   MASTER_URL  (required) e.g. wss://fnmdp.example.com/ingest
#
# Idempotent: re-running re-writes the compose + .env and `compose up -d`.
#
# Security note: this is a convenience script for throwaway rented boxes. It
# uses password SSH (sshpass) and disables host-key checking. Prefer SSH keys
# and known_hosts for anything you care about.
set -euo pipefail

HOSTS_FILE="${1:?usage: deploy-nodes.sh hosts.txt [N]}"
LIMIT="${2:-0}"
SSH_USER="${SSH_USER:-root}"
REGION="${REGION:-unknown}"
: "${MASTER_URL:?set MASTER_URL, e.g. wss://fnmdp.example.com/ingest}"
: "${NODE_TOKEN:?set NODE_TOKEN}"
: "${SSH_PASS:?set SSH_PASS}"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
COMPOSE_SRC="$SCRIPT_DIR/docker-compose.yml"

SSH_OPTS=(-o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o ConnectTimeout=15)

count=0
while read -r ip node_id _; do
  [[ -z "$ip" || "$ip" == \#* ]] && continue
  count=$((count + 1))
  if [[ "$LIMIT" -gt 0 && "$count" -gt "$LIMIT" ]]; then break; fi

  node_id_lc="$(echo "$node_id" | tr '[:upper:]' '[:lower:]')"
  echo "==> [$count] $ip  (NODE_ID=$node_id_lc)"

  # Ensure docker + compose plugin exist, then push compose + .env and start.
  sshpass -p "$SSH_PASS" ssh "${SSH_OPTS[@]}" "$SSH_USER@$ip" bash -s -- \
      "$node_id_lc" "$REGION" "$MASTER_URL" "$NODE_TOKEN" <<'REMOTE'
set -euo pipefail
NODE_ID="$1"; REGION="$2"; MASTER_URL="$3"; NODE_TOKEN="$4"
if ! command -v docker >/dev/null 2>&1; then
  curl -fsSL https://get.docker.com | sh
fi
mkdir -p /opt/fnmdp
cat > /opt/fnmdp/.env <<EOF
NODE_ID=${NODE_ID}
NODE_NAME=fnmdp-node-${NODE_ID}
REGION=${REGION}
MASTER_URL=${MASTER_URL}
NODE_TOKEN=${NODE_TOKEN}
EOF
chmod 600 /opt/fnmdp/.env
REMOTE

  # Copy the compose file and bring the stack up.
  sshpass -p "$SSH_PASS" scp "${SSH_OPTS[@]}" "$COMPOSE_SRC" "$SSH_USER@$ip:/opt/fnmdp/docker-compose.yml"
  sshpass -p "$SSH_PASS" ssh "${SSH_OPTS[@]}" "$SSH_USER@$ip" \
    'cd /opt/fnmdp && docker compose pull -q && docker compose up -d && docker compose ps'
  echo "==> [$count] $ip done"
done < "$HOSTS_FILE"

echo "Deployed $count host(s)."
