#!/bin/sh

# Get push gateway url and container image from env variable
if ([ ! -z "$PUSHGATEWAY_URL" ] && [ ! -z "$CONTAINER_IMAGE" ] && [ ! -z "$SD_PIPELINE_ID" ]); then
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
  cat <<EOF | curl -s -m 10 --data-binary @- "$PUSHGATEWAY_URL/metrics/job/containerd/instance/$5" &>/dev/null &
sd_build_status{image_name="$CONTAINER_IMAGE", pipeline_id="$SD_PIPELINE_ID", node="$NODE_ID", status="RUNNING", env="$SD_ENVIRONMENT"} 1
sd_build_image_pull_time_secs{image_name="$CONTAINER_IMAGE", pipeline_id="$SD_PIPELINE_ID", node="$NODE_ID", env="$SD_ENVIRONMENT"} $duration
sd_build_launcher_time_secs{image_name="$CONTAINER_IMAGE", pipeline_id="$SD_PIPELINE_ID", node="$NODE_ID", env="$SD_ENVIRONMENT"} $launcherduration
EOF
fi

echo "run launch"
# wrapper script for run build in multiple executors.
SD_TOKEN=`/opt/sd/launch --only-fetch-token --token "$1" --api-uri "$2" --store-uri "$3" --ui-uri "$6" --emitter /sd/emitter --build-timeout "$4" --cache-strategy "$7" --pipeline-cache-dir "$8" --job-cache-dir "$9" --event-cache-dir "${10}" --cache-compress "${11}" --cache-md5check "${12}" --cache-max-size-mb "${13}" "$5"` && (/opt/sd/launch --token "$SD_TOKEN" --api-uri "$2" --store-uri "$3" --ui-uri "$6" --emitter /sd/emitter --build-timeout "$4" --cache-strategy "$7" --pipeline-cache-dir "$8" --job-cache-dir "$9" --event-cache-dir "${10}" --cache-compress "${11}" --cache-md5check "${12}" --cache-max-size-mb "${13}" "$5" & /opt/sd/logservice --token "$SD_TOKEN" --emitter /sd/emitter --api-uri "$2" --store-uri "$3" --build "$5" & wait $(jobs -p))
