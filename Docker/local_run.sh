#!/bin/sh

/opt/sd/launch --local-mode --local-build-json "$1" --local-job-name "$2" --api-uri "$3" --emitter /sd/emitter 0 &
/opt/sd/logservice --local-mode --build-log-file "$4" --emitter /sd/emitter &
wait $(jobs -p)
