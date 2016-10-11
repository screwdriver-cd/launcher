FROM alpine:3.4
MAINTAINER The Screwdrivers <screwdriver.cd>

WORKDIR /opt/screwdriver

# Download Launcher, Log Service, and Tini setup
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
   && wget -q -O - https://github.com/screwdriver-cd/launcher/releases/latest \
      | egrep -o '/screwdriver-cd/launcher/releases/download/v[0-9.]*/launch' \
      | wget --base=http://github.com/ -i - -O launch \
   && chmod +x launch \
   # Download Log Service
   && wget -q -O - https://github.com/screwdriver-cd/log-service/releases/latest \
      | egrep -o '/screwdriver-cd/log-service/releases/download/v[0-9.]*/logservice' \
      | wget --base=http://github.com/ -i - -O logservice \
   && chmod +x logservice \
   # Download Tini Static
   && wget -q -O - https://github.com/krallin/tini/releases/latest \
      | egrep -o '/krallin/tini/releases/download/v[0-9.]*/tini-static' \
      | head -1 \
      | wget --base=http://github.com/ -i - -O tini-static \
   && wget -q -O - https://github.com/krallin/tini/releases/latest \
      | egrep -o '/krallin/tini/releases/download/v[0-9.]*/tini-static.asc' \
      | wget --base=http://github.com/ -i - -O tini-static.asc \
   && gpg --keyserver ha.pool.sks-keyservers.net --recv-keys 595E85A6B1B4779EA4DAAEC70B588DFF0527A9B7 \
   && gpg --verify tini-static.asc \
   && mv tini-static tini \
   && chmod +x tini \
   # Create FIFO
   && mkfifo emitter \
   # Cleanup packages
   && apk del --purge .build-dependencies

VOLUME /opt/screwdriver

# Set Entrypoint
ENTRYPOINT ["/opt/screwdriver/tini", "--", "/opt/screwdriver/launch"]
