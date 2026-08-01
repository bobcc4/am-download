#!/bin/sh
set -eu

if [ "$(id -u)" = "0" ]; then
    mkdir -p "$DATA_DIR" "$DOWNLOAD_DIR"

    # App state is small, so repair migrated files as well as the mount root.
    chown -R amdownload:amdownload "$DATA_DIR"
    chmod -R u+rwX "$DATA_DIR"

    # Downloads can be large; only new content needs the writable mount root.
    chown amdownload:amdownload "$DOWNLOAD_DIR"
    chmod u+rwx "$DOWNLOAD_DIR"

    exec setpriv --reuid=amdownload --regid=amdownload --init-groups -- "$@"
fi

exec "$@"
