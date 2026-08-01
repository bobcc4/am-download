#!/bin/sh
set -eu

files=/app/rootfs/data/data/com.apple.android.music/files
case " $* " in
  *" -L "*)
    echo "Enter your 2FA code into rootfs//data/data/com.apple.android.music/files/2fa.txt"
    echo "Waiting for input..."
    for _ in $(seq 1 100); do
      if [ -s "$files/2fa.txt" ]; then
        mkdir -p "$files/mpl_db"
        : > "$files/mpl_db/kvs.sqlitedb"
        : > "$files/fsi.pdat"
        exit 0
      fi
      sleep 0.1
    done
    exit 1
    ;;
esac

trap 'exit 0' TERM INT
while :; do sleep 1; done
