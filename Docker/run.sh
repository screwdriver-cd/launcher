#!/bin/sh

# wrapper script for run build in multiple executors.
SD_TOKEN=`/opt/sd/launch --only-fetch-token --token "$1" --api-uri "$2" --store-uri "$3" --ui-uri "$6" --emitter /sd/emitter --build-timeout "$4" --cache-strategy "$7" --pipeline-cache-dir "$8" --job-cache-dir "$9" --event-cache-dir "$10" "$5"` && (/opt/sd/launch --token "$SD_TOKEN" --api-uri "$2" --store-uri "$3" --ui-uri "$6" --emitter /sd/emitter --build-timeout "$4" --cache-strategy "$7" --pipeline-cache-dir "$8" --job-cache-dir "$9" --event-cache-dir "$10" "$5" & /opt/sd/logservice --token "$SD_TOKEN" --emitter /sd/emitter --api-uri "$2" --store-uri "$3" --build "$5" & wait $(jobs -p))
