FROM alpine:3.4
MAINTAINER The Screwdrivers <screwdriver.cd>

# Download Launcher, Log Service, and Tini setup
WORKDIR /opt/sd
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
      | egrep -o '/screwdriver-cd/launcher/releases/download/v[0-9.]*/launch\b' \
      | wget --base=http://github.com/ -i - -O launch \
   && chmod +x launch \
   # Download Log Service
   && wget -q -O - https://github.com/screwdriver-cd/log-service/releases/latest \
      | egrep -o '/screwdriver-cd/log-service/releases/download/v[0-9.]*/logservice\b' \
      | wget --base=http://github.com/ -i - -O logservice \
   && chmod +x logservice \
   # Download Meta CLI
   && wget -q -O - https://github.com/screwdriver-cd/meta-cli/releases/latest \
      | egrep -o '/screwdriver-cd/meta-cli/releases/download/v[0-9.]*/meta\b' \
      | wget --base=http://github.com/ -i - -O meta\
   && chmod +x meta\
   # Download Tini Static
   && wget -q -O - https://github.com/krallin/tini/releases/latest \
      | egrep -o '/krallin/tini/releases/download/v[0-9.]*/tini-static\b' \
      | head -1 \
      | wget --base=http://github.com/ -i - -O tini-static \
   && wget -q -O - https://github.com/krallin/tini/releases/latest \
      | egrep -o '/krallin/tini/releases/download/v[0-9.]*/tini-static.asc\b' \
      | wget --base=http://github.com/ -i - -O tini-static.asc \
   && gpg --keyserver ha.pool.sks-keyservers.net --recv-keys 595E85A6B1B4779EA4DAAEC70B588DFF0527A9B7 \
   && gpg --verify tini-static.asc \
   && rm tini-static.asc \
   && mv tini-static tini \
   && chmod +x tini \
   # Install Habitat
   && wget 'https://api.bintray.com/content/habitat/stable/linux/x86_64/hab-$latest-x86_64-linux.tar.gz?bt_package=hab-x86_64-linux' \
   && tar -C . -ozxvf hab-\$latest-x86_64-linux.tar.gz?bt_package=hab-x86_64-linux \
   && mkdir /opt/sd/bin \
   && mv /opt/sd/hab-*-x86_64-linux/hab /opt/sd/bin/hab \
   && chmod +x /opt/sd/bin/hab \
   && rm -rf hab-* \
   # Download sd-step
   && wget -q -O - https://github.com/screwdriver-cd/sd-step/releases/latest \
      | egrep -o '/screwdriver-cd/sd-step/releases/download/v[0-9.]*/sd-step\b' \
      | wget --base=http://github.com/ -i - -O sd-step\
   && chmod +x sd-step \
   # Create FIFO
   && mkfifo -m 666 emitter \
   # Cleanup packages
   && apk del --purge .build-dependencies

VOLUME /opt/sd

# Set Entrypoint
ENTRYPOINT ["/opt/sd/tini", "--", "/opt/sd/launch"]
