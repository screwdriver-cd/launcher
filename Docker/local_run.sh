#!/bin/sh

# Create FIFO for emitter
mkdir -p /sd
mkfifo -m 666 /sd/emitter

# Because launcher doesn't use buildID in local-mode, pass 0 to launcher as dummy.
/opt/sd/launch --local-mode --local-build-json "$1" --local-job-name "$2" --api-uri "$3" --store-uri "$4" --emitter /sd/emitter 0 &
/opt/sd/logservice --local-mode --build-log-file "$5" --emitter /sd/emitter &
wait $(jobs -p)
