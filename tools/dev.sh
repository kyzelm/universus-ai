#!/usr/bin/env bash
# Splits the current kitty tab for the dev loop: lazydocker over client | server, Postgres in compose.
set -euo pipefail

root=$(cd "$(dirname "$0")/.." && pwd)
cd "$root"

if ! kitty @ ls >/dev/null 2>&1; then
  echo "dev: kitty remote control is off; add this to kitty.conf and restart kitty:" >&2
  echo '  allow_remote_control socket-only' >&2
  echo '  listen_on unix:${XDG_RUNTIME_DIR}/kitty' >&2
  exit 1
fi
if ! docker info >/dev/null 2>&1; then
  echo "dev: docker daemon is down, run: sudo systemctl start docker" >&2
  exit 1
fi
docker compose up -d --wait db

# Matched by this window, so switching tabs while the db starts changes nothing.
tab="window_id:$KITTY_WINDOW_ID"
kitty @ goto-layout --match "$tab" splits
client=$(kitty @ launch --match "$tab" --next-to "id:$KITTY_WINDOW_ID" --location=hsplit --keep-focus \
  --title client --hold --cwd "$root/client" pnpm dev)
kitty @ launch --match "$tab" --next-to "id:$client" --location=vsplit --keep-focus \
  --title server --hold --cwd "$root" \
  --env DATABASE_URL=postgres://postgres:dev@localhost:5432/universus go run ./server
exec lazydocker
