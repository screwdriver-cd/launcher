#!/bin/sh
set -e

args=$@

# Trap these SIGNALs HUP INT QUIT TERM EXIT and update build status to failure
trap cleanUp HUP INT QUIT TERM EXIT

cleanUp () {
  if [ $? -ne 0 ]; then
    token=$(eval echo $args | awk '{ print $2 }')
    apiUri=$(eval echo $args | awk '{ print $3 }')
    storeUri=$(eval echo $args | awk '{ print $4 }')
    timeout=$(eval echo $args | awk '{ print $5 }')
    buildId=$(eval echo $args | awk '{ print $6 }')
    uiUri=$(eval echo $args | awk '{ print $7 }')
    cacheStrategy=$(eval echo $args | awk '{ print $8 }')
    pipelineCacheDir=$(eval echo $args | awk '{ print $9 }')
    jobCacheDir=$(eval echo $args | awk '{ print $10 }')
    eventCacheDir=$(eval echo $args | awk '{ print $11 }')
    cacheCompress=$(eval echo $args | awk '{ print $12 }')
    cacheMd5Chk=$(eval echo $args | awk '{ print $13 }')
    cacheMaxSizeMB=$(eval echo $args | awk '{ print $14 }')
    cacheMaxGoThreads=$(eval echo $args | awk '{ print $15 }')

    /opt/sd/launch --container-error --token $token --api-uri $apiUri --store-uri $storeUri --ui-uri $uiUri --emitter /sd/emitter --build-timeout $timeout --cache-strategy $cacheStrategy --pipeline-cache-dir $pipelineCacheDir --job-cache-dir $jobCacheDir --event-cache-dir $eventCacheDir --cache-compress $cacheCompress --cache-md5check $cacheMd5Chk --cache-max-size-mb $cacheMaxSizeMB --cache-max-go-threads $cacheMaxGoThreads $buildId

    exit 1
  fi
}

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

binary_exists() {
    smart_run command -v "$1" >/dev/null 2>&1
}

echo 'sudo available in Container?'
smart_run whoami

echo 'Symlink hab cache'
date

if [ "$SD_HAB_ENABLED" = "true" ]; then
    smart_run mkdir -p -m 777 /hab/bin || echo 'Failed to create /hab/bin'
    smart_run ln -sf /opt/sd/bin/hab /hab/bin/hab || echo 'Failed to symlink hab bin'
    smart_run mkdir -p -m 777 /hab/pkgs || echo 'Failed to create /hab/pkgs/'
    smart_run mkdir -p -m 777 /hab/pkgs/core  || echo 'Failed to create /hab/pkgs/core'
    # this cmd will create dirs for all hab pkgs in this format: /hab/pkgs/core/curl/7.54.1
    find /opt/sd/hab/pkgs/core -mindepth 2 -maxdepth 2 -exec sh -c 'mkdir -p `echo $1 | sed "s/\/opt\/sd//"`' -- {} \; || echo 'Failed to create /hab/pkgs/core/*'
    # this cmd will symlink the specific version: ln -s  /opt/sd/hab/pkgs/core/curl/7.54.1/20181008145326 /hab/pkgs/core/curl/7.54.1
    find /opt/sd/hab/pkgs/core -mindepth 3 -maxdepth 3 -exec sh -c 'ln -s $1 `dirname $1 | sed "s/\/opt\/sd//"`' -- {} \; || echo 'Failed to symlink hab cache'

    # Create directory for hab pkg binlink destination
    smart_run mkdir -p /usr/sd/bin

    # Binlinking bash from core/bash into /bin
    if ! binary_exists bash; then
        smart_run /opt/sd/bin/hab pkg binlink core/bash bash || echo 'Failed to symlink bash'
    fi

# Binlinking jq from core/jq into /usr/sd/bin
    if ! binary_exists jq; then
        smart_run /opt/sd/bin/hab pkg binlink -d /usr/sd/bin core/jq-static jq || echo 'Failed to symlink jq'
    fi
fi

echo 'Creating workspace and log pipe'
date
# Create FIFO for emitter
# https://github.com/screwdriver-cd/screwdriver/issues/979
smart_run mkdir -p -m 777 /sd
smart_run mkfifo -m 666 /sd/emitter

echo 'Waiting for log pipe to be ready'
date
# Make sure everything is ready
while ! [ -p /sd/emitter ]
do
    sleep 1
done

echo 'Symlink hab cache, log pipe is ready'
date

if [ -x "$(command -v /bin/bash)" ]; then SD_LAUNCHER_SHELL="/bin/bash"; fi
SD_LAUNCHER_SHELL="${SD_LAUNCHER_SHELL:-/bin/sh}"

# exec run.sh using dumbinit
exec /opt/sd/dumb-init -- $SD_LAUNCHER_SHELL -c "$@"
