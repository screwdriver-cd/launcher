#!/bin/sh

# push data to prometheus via pushgateway url
pushToPrometheus() {
  curl -s -m 10 --data-binary @- "$PUSHGATEWAY_URL/metrics/job/containerd/instance/$1" &>/dev/null &
}

# Get push gateway url and container image from env variable
if [[ ! -z "$PUSHGATEWAY_URL" ]] && [[ ! -z "$CONTAINER_IMAGE" ]] && [[ ! -z "$SD_PIPELINE_ID" ]]; then
  echo "Push metrics to prometheus"
  cat <<EOF | pushToPrometheus "$5"
sd_build_image{image_name="$CONTAINER_IMAGE", pipeline_id="$SD_PIPELINE_ID", node="$NODE_ID"} 1
EOF
fi

# wrapper script for run build in multiple executors.
SD_TOKEN=`/opt/sd/launch --only-fetch-token --token "$1" --api-uri "$2" --store-uri "$3" --ui-uri "$6" --emitter /sd/emitter --build-timeout "$4" --cache-strategy "$7" --pipeline-cache-dir "$8" --job-cache-dir "$9" --event-cache-dir "${10}" --cache-compress "${11}" --cache-md5check "${12}" --cache-max-size-mb "${13}" "$5"` && (/opt/sd/launch --token "$SD_TOKEN" --api-uri "$2" --store-uri "$3" --ui-uri "$6" --emitter /sd/emitter --build-timeout "$4" --cache-strategy "$7" --pipeline-cache-dir "$8" --job-cache-dir "$9" --event-cache-dir "${10}" --cache-compress "${11}" --cache-md5check "${12}" --cache-max-size-mb "${13}" "$5" & /opt/sd/logservice --token "$SD_TOKEN" --emitter /sd/emitter --api-uri "$2" --store-uri "$3" --build "$5" & wait $(jobs -p))
