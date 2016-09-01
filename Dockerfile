FROM alpine:3.4
MAINTAINER The Screwdrivers <screwdriver.cd>

WORKDIR /opt/screwdriver

# Download Launcher and Tini setup
WORKDIR /opt/screwdriver
RUN set -x \
   # Alpine ships with musl instead of glibc (this fixes the symlink)
   && mkdir /lib64 \
   && ln -s /lib/libc.musl-x86_64.so.1 /lib64/ld-linux-x86-64.so.2 \
   # Also, missing https for some magic reason
   && apk add --no-cache --update ca-certificates \
   && apk add --virtual .build-dependencies wget \
   && apk add --virtual .build-dependencies gpgme \
   # Download Launcher
   && wget $(wget -q -O - https://api.github.com/repos/screwdriver-cd/launcher/releases/latest \
       | awk '/browser_download_url/ { print $2 }' \
       | sed 's/"//g') \
   && chmod +x launch \
   # Download Tini Static
   && wget $(wget -q -O - https://api.github.com/repos/krallin/tini/releases/latest \
       | awk '/browser_download_url/ { print $2 }' \
       | sed 's/"//g' \
       | grep 'static') \
   && gpg --keyserver ha.pool.sks-keyservers.net --recv-keys 0527A9B7 \
   && gpg --verify tini-static.asc \
   && mv tini-static tini \
   && chmod +x tini \
   # Cleanup packages
   && apk del --purge .build-dependencies

VOLUME /opt/screwdriver

# Set Entrypoint
ENTRYPOINT ["/opt/screwdriver/tini", "--", "/opt/screwdriver/launch"]
