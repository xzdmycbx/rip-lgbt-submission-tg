#!/bin/sh
# entrypoint.sh — runs as root inside the container, makes sure the
# bind-mounted /data is writable by the unprivileged "rip" user, then
# drops privileges and exec's the real command (the server binary).
#
# This lets users `docker compose up` without first chown'ing the host
# data directory.

set -e

DATA_DIR="${DATA_DIR:-/data}"
RUN_USER="${RUN_USER:-rip}"

# Resolve the target uid/gid by name. busybox `id -u <name>` works on alpine.
TARGET_UID="$(id -u "$RUN_USER")"
TARGET_GID="$(id -g "$RUN_USER")"

if [ ! -d "$DATA_DIR" ]; then
  mkdir -p "$DATA_DIR"
fi

# Only touch ownership when the directory is not already owned by the
# expected uid. This avoids triggering recursive writes on an FS that may
# have lots of files (uploads grow over time).
CURRENT_UID="$(stat -c '%u' "$DATA_DIR" 2>/dev/null || echo 0)"
if [ "$CURRENT_UID" != "$TARGET_UID" ]; then
  echo "entrypoint: chown $DATA_DIR -> $TARGET_UID:$TARGET_GID (was uid=$CURRENT_UID)"
  chown -R "$TARGET_UID:$TARGET_GID" "$DATA_DIR"
fi

# Hand off to dumb-init -> su-exec -> real command.
exec /usr/bin/dumb-init -- /sbin/su-exec "$RUN_USER" "$@"
