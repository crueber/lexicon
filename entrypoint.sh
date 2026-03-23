#!/bin/sh
set -e

# Support custom USER_ID and GROUP_ID for file permission compatibility
# with the host filesystem (common in self-hosted/homelab setups).
if [ -n "$USER_ID" ] && [ -n "$GROUP_ID" ]; then
    # Modify the lexicon user/group to match the requested IDs.
    deluser lexicon 2>/dev/null || true
    delgroup lexicon 2>/dev/null || true
    addgroup -g "$GROUP_ID" -S lexicon
    adduser -u "$USER_ID" -G lexicon -S -D -H lexicon
fi

# Ensure data directories exist and are owned by the lexicon user.
for dir in /app/data /books /bookdrop; do
    mkdir -p "$dir"
    chown lexicon:lexicon "$dir" 2>/dev/null || true
done

# Run the command as the lexicon user.
exec su-exec lexicon "$@" 2>/dev/null || exec "$@"
