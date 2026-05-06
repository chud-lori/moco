#!/usr/bin/env bash
# Pull latest code, rebuild the container, restart, and verify health.
# Run this from the repo root on the deployment VM.
#
#   ./deploy.sh
set -euo pipefail

# Move into the script's own directory so it works no matter where it's called from.
cd "$(dirname "$0")"

if [[ ! -f .env ]]; then
    echo "✖ .env is missing. Copy .env.example and fill it in first."
    exit 1
fi

# Pull host port out of .env (used for the post-deploy health check).
PORT=$(grep -E '^MOCO_PORT=' .env 2>/dev/null | head -1 | cut -d= -f2- | tr -d '"' | xargs || true)
PORT=${PORT:-8666}

echo "==> [1/4] git pull"
git pull --ff-only

echo
echo "==> [2/4] docker compose up -d --build"
docker compose up -d --build

echo
echo "==> [3/4] waiting for health check on http://127.0.0.1:${PORT}"
ok=0
for i in {1..30}; do
    if curl -fsS "http://127.0.0.1:${PORT}/api/v1/health" >/dev/null 2>&1; then
        ok=1
        break
    fi
    sleep 1
done

if [[ $ok -ne 1 ]]; then
    echo "✖ health check timed out after 30s. Recent logs:"
    docker logs --tail 50 moco
    exit 1
fi

echo "✓ moco is up"
echo
echo "==> [4/4] last 20 log lines"
docker logs --tail 20 moco
