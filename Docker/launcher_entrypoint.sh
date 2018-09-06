#!/bin/sh

smart_run () {
    if ! $I_AM_ROOT; then
        sudo $@
    else
        $@
    fi
}

# Create FIFO for emitter
# https://github.com/screwdriver-cd/screwdriver/issues/979
smart_run mkfifo -m 666 /opt/sd/emitter

# Entrypoint
/opt/sd/tini -- /bin/sh -c "$@"
