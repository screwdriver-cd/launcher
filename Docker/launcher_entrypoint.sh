#!/bin/sh
set -e

clean_up () {
  if [ $? -ne 0 ]; then
    /opt/sd/launch  --run-teardown --token "${2}" --api-uri "${3}" --store-uri "${4}" --ui-uri "${7}" --emitter /sd/emitter --build-timeout "${5}" --cache-strategy "${8}" --pipeline-cache-dir "${9}" --job-cache-dir "${10}" --event-cache-dir "${11}" --cache-compress "${12}" --cache-md5check "${13}" --cache-max-size-mb "${14}" --exit-code "500" --cache-max-go-threads "${15}" "${6}"
   exit 1
  fi
}

# Trap these SIGNALs and run teardown
trap 'clean_up $@'  SIGINT SIGTERM EXIT SIGQUIT SIGHUP

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

echo 'sudo available in Container?'
smart_run whoami

echo 'Symlink hab cache'
date
smart_run mkdir -p -m 777 /hab/bin || echo 'Failed to create /hab/bin'
smart_run ln -sf /opt/sd/bin/hab /hab/bin/hab || echo 'Failed to symlink hab bin'
smart_run mkdir -p -m 777 /hab/pkgs || echo 'Failed to create /hab/pkgs/'
smart_run mkdir -p -m 777 /hab/pkgs/core  || echo 'Failed to create /hab/pkgs/core'
# this cmd will create dirs for all hab pkgs in this format: /hab/pkgs/core/curl/7.54.1
find /opt/sd/hab/pkgs/core -mindepth 2 -maxdepth 2 -exec sh -c 'mkdir -p `echo $1 | sed "s/\/opt\/sd//"`' -- {} \; || echo 'Failed to create /hab/pkgs/core/*'
# this cmd will symlink the specific version: ln -s  /opt/sd/hab/pkgs/core/curl/7.54.1/20181008145326 /hab/pkgs/core/curl/7.54.1
find /opt/sd/hab/pkgs/core -mindepth 3 -maxdepth 3 -exec sh -c 'ln -s $1 `dirname $1 | sed "s/\/opt\/sd//"`' -- {} \; || echo 'Failed to symlink hab cache'

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

# Entrypoint
exec /opt/sd/tini -- /bin/sh -c "$@"
