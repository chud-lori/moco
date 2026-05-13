#!/usr/bin/env bash
# One-shot moco snapshot for server migrations. Captures the SQLite DB,
# any local book files, the R2 bucket (if MOCO_STORAGE=r2), and a redacted
# copy of .env. Briefly stops the docker container so SQLite isn't being
# written to mid-copy — ~5s of downtime.
#
# Usage:
#   ./backup.sh                                # run on the host moco is on
#   ./backup.sh --from user@host:/path/to/moco # ssh to that host, back up,
#                                              # then rsync the tarball down
#
# Output:
#   ./backups/moco-YYYYMMDD-HHMMSSZ.tar.gz
#
# Restore (target host, fresh moco checkout in place):
#   docker compose down
#   rm -rf ./var && mkdir -p ./var
#   tar -xzf moco-*.tar.gz -C /tmp/moco-restore
#   cp -a /tmp/moco-restore/var/. ./var/
#   # If using R2: rclone copy /tmp/moco-restore/r2/ <new-bucket>:
#   # Rebuild .env from /tmp/moco-restore/env.redacted, filling in <redacted>s
#   docker compose up -d --build

set -euo pipefail

REMOTE=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --from)
      REMOTE="${2:?--from needs user@host:/path}"
      shift 2
      ;;
    -h|--help)
      sed -n '2,/^$/p' "$0" | sed 's/^# \?//'
      exit 0
      ;;
    *)
      echo "✖ unknown arg: $1" >&2
      exit 1
      ;;
  esac
done

# ---- Remote mode: SSH in, run ourselves there, rsync the tarball back ----
if [[ -n "$REMOTE" ]]; then
  HOST="${REMOTE%%:*}"
  REMOTE_PATH="${REMOTE#*:}"
  if [[ "$HOST" == "$REMOTE" || -z "$REMOTE_PATH" ]]; then
    echo "✖ --from must be user@host:/path/to/moco" >&2
    exit 1
  fi
  echo "==> remote backup on ${HOST} (path: ${REMOTE_PATH})"
  scp -q "$0" "${HOST}:/tmp/moco-backup.sh"
  ssh "$HOST" "cd '${REMOTE_PATH}' && bash /tmp/moco-backup.sh"
  REMOTE_TARBALL=$(ssh "$HOST" "ls -t '${REMOTE_PATH}'/backups/moco-*.tar.gz | head -1")
  mkdir -p ./backups
  echo "==> rsync ${HOST}:${REMOTE_TARBALL} → ./backups/"
  rsync -avz --progress "${HOST}:${REMOTE_TARBALL}" ./backups/
  ssh "$HOST" "rm -f /tmp/moco-backup.sh"
  echo "✓ remote backup landed in ./backups/$(basename "${REMOTE_TARBALL}")"
  exit 0
fi

# ---- Local mode ----
cd "$(dirname "$0")"

if [[ ! -f .env ]]; then
  echo "✖ .env missing — run from the moco repo root" >&2
  exit 1
fi

# .env helper: grab a key, strip quotes/whitespace, default to empty.
env_get() {
  grep -E "^${1}=" .env 2>/dev/null | head -1 | cut -d= -f2- | tr -d '"' | xargs || true
}

STORAGE=$(env_get MOCO_STORAGE)
STORAGE="${STORAGE:-local}"
STAMP=$(date -u +%Y%m%d-%H%M%SZ)
STAGE=$(mktemp -d 2>/dev/null || mktemp -d -t moco-backup)
OUT_DIR="./backups"
TARBALL="${OUT_DIR}/moco-${STAMP}.tar.gz"
mkdir -p "$OUT_DIR"

echo "==> moco backup ${STAMP}"
echo "    storage: ${STORAGE}"

# Stop the container if running so SQLite WAL is checkpointed cleanly.
WAS_RUNNING=0
if command -v docker >/dev/null 2>&1 && docker ps --format '{{.Names}}' 2>/dev/null | grep -q '^moco$'; then
  WAS_RUNNING=1
  echo "==> stopping moco container"
  docker compose stop moco >/dev/null
fi

# Ensure we always restart and clean up, even on failure.
cleanup() {
  if [[ $WAS_RUNNING -eq 1 ]]; then
    docker compose start moco >/dev/null 2>&1 || true
  fi
  rm -rf "$STAGE"
}
trap cleanup EXIT

# var/ holds the SQLite DB + (for MOCO_STORAGE=local) book files.
if [[ ! -d ./var ]]; then
  echo "✖ ./var missing — nothing to snapshot" >&2
  exit 1
fi
echo "==> copy ./var → stage"
cp -a ./var "${STAGE}/var"

# Redact secrets from .env so a leaked backup doesn't leak creds. Restore
# by hand-filling these on the new host.
echo "==> redact .env"
awk -F= '
  /^[[:space:]]*#/ || /^[[:space:]]*$/ { print; next }
  $1 ~ /(SECRET|TOKEN|PASSWORD|KEY|CLIENT_ID)/ { print $1 "=<redacted>"; next }
  { print }
' .env > "${STAGE}/env.redacted"

# R2 bucket dump, if applicable. Uses inline creds from .env — no rclone.conf
# setup needed. Requires rclone on PATH.
if [[ "$STORAGE" == "r2" ]]; then
  if ! command -v rclone >/dev/null 2>&1; then
    echo "  ⚠ R2 backend but rclone not installed — skipping bucket dump"
    echo "    install: brew install rclone (mac) / apt install rclone (deb)"
  else
    ACCOUNT=$(env_get MOCO_R2_ACCOUNT_ID)
    AK=$(env_get MOCO_R2_ACCESS_KEY_ID)
    SK=$(env_get MOCO_R2_SECRET_ACCESS_KEY)
    BUCKET=$(env_get MOCO_R2_BUCKET)
    if [[ -z "$ACCOUNT" || -z "$AK" || -z "$SK" || -z "$BUCKET" ]]; then
      echo "  ⚠ R2 creds incomplete in .env — skipping bucket dump"
    else
      echo "==> rclone copy r2:${BUCKET} → stage/r2"
      rclone copy ":s3:${BUCKET}" "${STAGE}/r2" \
        --s3-provider Cloudflare \
        --s3-access-key-id "$AK" \
        --s3-secret-access-key "$SK" \
        --s3-endpoint "https://${ACCOUNT}.r2.cloudflarestorage.com" \
        --transfers 8 \
        --stats=10s
    fi
  fi
fi

# Restart now — packing the tarball doesn't need the container down.
if [[ $WAS_RUNNING -eq 1 ]]; then
  echo "==> restart moco container"
  docker compose start moco >/dev/null
  WAS_RUNNING=0
fi

echo "==> pack ${TARBALL}"
tar -czf "$TARBALL" -C "$STAGE" .

SIZE=$(du -h "$TARBALL" | cut -f1)
echo "✓ ${TARBALL} (${SIZE})"
