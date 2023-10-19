#!/bin/bash -e

# Trap these SIGNALs and run teardown
trap 'trap_handler $@' INT TERM

trap_handler () {
    code=$?
    if [ $code -ne 0 ]; then
        echo "Exit code:$code received in run.sh, waiting for $SD_TERMINATION_GRACE_PERIOD_SECONDS seconds"
        # this trap does not wait for launcher SIGTERM processing completion 
        # so add sleep until timeoutgraceperiodseconds is elapsed
        sleep $SD_TERMINATION_GRACE_PERIOD_SECONDS
    fi
}

# Get push gateway url and container image from env variable
if ([ ! -z "$SD_PUSHGATEWAY_URL" ] && [ ! -z "$CONTAINER_IMAGE" ] && [ ! -z "$SD_PIPELINE_ID" ]); then
  ts=`date "+%s"`
  export SD_BUILD_START_TS=$ts
  echo "push build image metrics to prometheus"
  launcherstartts=$(cat /workspace/metrics | grep launcher_start_ts | awk -F':' '{print $2}')
  [ -z "$launcherstartts" ] && launcherstartts=$ts
  export SD_LAUNCHER_START_TS=$launcherstartts
  launcherendts=$(cat /workspace/metrics | grep launcher_end_ts | awk -F':' '{print $2}')
  [ -z "$launcherendts" ] && launcherendts=$ts
  export SD_LAUNCHER_END_TS=$launcherendts
  duration=$(($ts - $launcherendts))
  launcherduration=$(($launcherendts - $launcherstartts))
  cat <<EOF | curl -s -m 10 --data-binary @- "$SD_PUSHGATEWAY_URL/metrics/job/containerd/instance/$5" &>/dev/null &
sd_build_status{image_name="$CONTAINER_IMAGE", pipeline_id="$SD_PIPELINE_ID", node="$NODE_ID", status="RUNNING", prefix="$SD_BUILD_PREFIX"} 1
sd_build_image_pull_time_secs{image_name="$CONTAINER_IMAGE", pipeline_id="$SD_PIPELINE_ID", node="$NODE_ID", prefix="$SD_BUILD_PREFIX"} $duration
sd_build_launcher_time_secs{image_name="$CONTAINER_IMAGE", pipeline_id="$SD_PIPELINE_ID", node="$NODE_ID", prefix="$SD_BUILD_PREFIX"} $launcherduration
EOF
fi

echo "run launch"
# wrapper script for run build in multiple executors.
if [ "$SD_AWS_INTEGRATION" = "true" ]; then
  # use environment variables from aws codebuild executor
  SD_TOKEN=`/opt/sd/launch --only-fetch-token --token "$TOKEN" --api-uri "$API" --store-uri "$STORE" --ui-uri "$UI" --emitter /sd/emitter --build-timeout "$TIMEOUT" --cache-strategy "$7" --pipeline-cache-dir "$8" --job-cache-dir "$9" --event-cache-dir "${10}" --cache-compress "${11}" --cache-md5check "${12}" --cache-max-size-mb "${13}" --cache-max-go-threads "${14}" "$SDBUILDID"` && (/opt/sd/launch --token "$SD_TOKEN" --api-uri "$API" --store-uri "$STORE" --ui-uri "$UI" --emitter /sd/emitter --build-timeout "$TIMEOUT" --cache-strategy "$7" --pipeline-cache-dir "$8" --job-cache-dir "$9" --event-cache-dir "${10}" --cache-compress "${11}" --cache-md5check "${12}" --cache-max-size-mb "${13}" --cache-max-go-threads "${14}" "$SDBUILDID" & /opt/sd/logservice --token "$SD_TOKEN" --emitter /sd/emitter --api-uri "$API" --store-uri "$STORE" --build "$SDBUILDID" & wait $(jobs -p))
else
  SD_TOKEN=`/opt/sd/launch --only-fetch-token --token "$1" --api-uri "$2" --store-uri "$3" --ui-uri "$6" --emitter /sd/emitter --build-timeout "$4" --cache-strategy "$7" --pipeline-cache-dir "$8" --job-cache-dir "$9" --event-cache-dir "${10}" --cache-compress "${11}" --cache-md5check "${12}" --cache-max-size-mb "${13}" --cache-max-go-threads "${14}" "$5"` && (/opt/sd/launch --token "$SD_TOKEN" --api-uri "$2" --store-uri "$3" --ui-uri "$6" --emitter /sd/emitter --build-timeout "$4" --cache-strategy "$7" --pipeline-cache-dir "$8" --job-cache-dir "$9" --event-cache-dir "${10}" --cache-compress "${11}" --cache-md5check "${12}" --cache-max-size-mb "${13}" --cache-max-go-threads "${14}" "$5" & /opt/sd/logservice --token "$SD_TOKEN" --emitter /sd/emitter --api-uri "$2" --store-uri "$3" --build "$5" & wait $(jobs -p))
fi

if [ -r /tmp/sd_event.json ]; then
  cat /tmp/sd_event.json
fi
