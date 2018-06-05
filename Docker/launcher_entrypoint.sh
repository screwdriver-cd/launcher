#!/bin/sh

# Create FIFO for emitter
# https://github.com/screwdriver-cd/screwdriver/issues/979
mkfifo -m 666 /opt/sd/emitter

# Entrypoint
/opt/sd/tini -- /opt/sd/launch "$@"
