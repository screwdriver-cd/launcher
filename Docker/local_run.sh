#!/bin/sh

I_AM_ROOT=false

user=`whoami`
echo "User: $user"

if [ "$user" = "root" ]; then
    I_AM_ROOT=true 
fi

smart_run () {
    if ! $I_AM_ROOT; then
        sudo $@
    else
        $@
    fi
}

# Create FIFO for emitter
mkdir -p /sd
mkfifo -m 666 /sd/emitter

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


# Because launcher doesn't use buildID in local-mode, pass 0 to launcher as dummy.
/opt/sd/launch --local-mode --local-build-json "$1" --local-job-name "$2" --api-uri "$3" --store-uri "$4" --emitter /sd/emitter 0 &
/opt/sd/logservice --local-mode --build-log-file "$5" --emitter /sd/emitter &
wait $(jobs -p)
