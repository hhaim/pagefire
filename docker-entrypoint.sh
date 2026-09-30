#!/bin/sh
set -eu

# A mounted Docker volume can hide the ownership set on /data in the image.
# Repair it before SQLite creates the database, then run the app unprivileged.
chown -R pagefire:pagefire /data
exec gosu pagefire pagefire "$@"
