#!/bin/bash -e

# Trap these SIGNALs and run teardown
trap 'trap_handler $@' INT TERM

trap_handler () {
    code=$?
    if [ $code -ne 0 ]; then
        echo "Exit code:$code received, run command"
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
SD_TOKEN=`/opt/sd/launch --only-fetch-token --token "$1" --api-uri "$2" --store-uri "$3" --ui-uri "$6" --emitter /sd/emitter --build-timeout "$4" --cache-strategy "$7" --pipeline-cache-dir "$8" --job-cache-dir "$9" --event-cache-dir "${10}" --cache-compress "${11}" --cache-md5check "${12}" --cache-max-size-mb "${13}" --cache-max-go-threads "${14}" "$5"` && (/opt/sd/launch --token "$SD_TOKEN" --api-uri "$2" --store-uri "$3" --ui-uri "$6" --emitter /sd/emitter --build-timeout "$4" --cache-strategy "$7" --pipeline-cache-dir "$8" --job-cache-dir "$9" --event-cache-dir "${10}" --cache-compress "${11}" --cache-md5check "${12}" --cache-max-size-mb "${13}" --cache-max-go-threads "${14}" "$5" & /opt/sd/logservice --token "$SD_TOKEN" --emitter /sd/emitter --api-uri "$2" --store-uri "$3" --build "$5" & wait $(jobs -p))