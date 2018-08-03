#!/bin/sh

# wrapper script for run build in multiple executors.
SD_TOKEN=`/opt/sd/launch --only-fetch-token --token "$1" --api-uri "$2" --store-uri "$3" --emitter /sd/emitter --build-timeout "$4" "$5"` && (/opt/sd/launch --api-uri "$2" --store-uri "$3" --emitter /sd/emitter --build-timeout "$4" "$5" & /opt/sd/logservice --emitter /sd/emitter --api-uri "$2" --store-uri "$3" --build "$5" & wait $(jobs -p))
