#!/bin/sh

I_AM_ROOT=false

if [ `whoami` = "root" ]; then
    I_AM_ROOT=true
fi

smart_run () {
    if ! $I_AM_ROOT; then
        sudo $@
    else
        $@
    fi
}

# Create FIFO for emitter
# https://github.com/screwdriver-cd/screwdriver/issues/979
smart_run mkdir -p /sd
smart_run mkfifo -m 666 /sd/emitter

# Entrypoint
/opt/sd/tini -- /bin/sh -c "$@"
