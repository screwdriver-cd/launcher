FROM alpine:3.4
MAINTAINER The Screwdrivers <screwdriver.cd>

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
      | egrep -o '/screwdriver-cd/launcher/releases/download/v[0-9.]*/launcher_linux_amd64' \
      | wget --base=http://github.com/ -i - -O launch \
   && chmod +x launch \
   # Download Log Service
   && wget -q -O - https://github.com/screwdriver-cd/log-service/releases/latest \
      | egrep -o '/screwdriver-cd/log-service/releases/download/v[0-9.]*/log-service_linux_amd64' \
      | wget --base=http://github.com/ -i - -O logservice \
   && chmod +x logservice \
   # Download Meta CLI
   && wget -q -O - https://github.com/screwdriver-cd/meta-cli/releases/latest \
      | egrep -o '/screwdriver-cd/meta-cli/releases/download/v[0-9.]*/meta-cli_linux_amd64' \
      | wget --base=http://github.com/ -i - -O meta \
   && chmod +x meta \
   # Download sd-step
   && wget -q -O - https://github.com/screwdriver-cd/sd-step/releases/latest \
      | egrep -o '/screwdriver-cd/sd-step/releases/download/v[0-9.]*/sd-step_linux_amd64' \
      | wget --base=http://github.com/ -i - -O sd-step \
   && chmod +x sd-step \
   # Download sd-cmd
   && wget -q -O - https://github.com/screwdriver-cd/sd-cmd/releases/latest \
      | egrep -o '/screwdriver-cd/sd-cmd/releases/download/v[0-9.]*/sd-cmd_linux_amd64' \
      | wget --base=http://github.com/ -i - -O sd-cmd \
   && chmod +x sd-cmd \

   # Download Tini Static
   && wget -q -O - https://github.com/krallin/tini/releases/latest \
      | egrep -o '/krallin/tini/releases/download/v[0-9.]*/tini-static' \
      | head -1 \
      | wget --base=http://github.com/ -i - -O tini-static \
   && wget -q -O - https://github.com/krallin/tini/releases/latest \
      | egrep -o '/krallin/tini/releases/download/v[0-9.]*/tini-static.asc' \
      | wget --base=http://github.com/ -i - -O tini-static.asc \
   && found=''; \
      ( \
      gpg --no-tty --keyserver pgp.mit.edu --recv-keys 595E85A6B1B4779EA4DAAEC70B588DFF0527A9B7 || \
      gpg --no-tty --keyserver keyserver.pgp.com --recv-keys 595E85A6B1B4779EA4DAAEC70B588DFF0527A9B7 || \
      gpg --no-tty --keyserver ha.pool.sks-keyservers.net --recv-keys 595E85A6B1B4779EA4DAAEC70B588DFF0527A9B7 \
      ) \
   && found=yes && break; \
     test -z "$found" && echo >&2 "error: failed to fetch GPG key 595E85A6B1B4779EA4DAAEC70B588DFF0527A9B7" && exit 1; \
     gpg --verify tini-static.asc  \
   && rm tini-static.asc \
   && mv tini-static tini \
   && chmod +x tini \

   # Install Habitat
   && mkdir -p /hab/bin /opt/sd/bin \
   # Download Habitat Binary
   && wget -O hab.tar.gz 'https://api.bintray.com/content/habitat/stable/linux/x86_64/hab-$latest-x86_64-linux.tar.gz?bt_package=hab-x86_64-linux' \
   && tar -C . -ozxvf hab.tar.gz \
   && mv hab-*/hab /hab/bin/hab \
   && chmod +x /hab/bin/hab \
   # @TODO Remove this, I don't think it belongs here.  We should use /hab/bin/hab instead.
   && cp /hab/bin/hab /opt/sd/bin/hab \
   # Install Habitat packages
   && /hab/bin/hab pkg install core/bash core/git core/zip \
   # Cleanup Habitat Files
   && rm -rf /hab/cache /opt/sd/hab.tar.gz /opt/sd/hab-* \
   # Cleanup docs and man pages (how could this go wrong)
   && find /hab -name doc -exec rm -r {} + \
   && find /hab -name docs -exec rm -r {} + \
   && find /hab -name man -exec rm -r {} + \

   # Create FIFO
   && mkfifo -m 666 emitter \
   # Cleanup packages
   && apk del --purge .build-dependencies

# Copy optional entrypoint script to the image
COPY Docker/launcher_entrypoint.sh /opt/sd/launcher_entrypoint.sh

# Copy wrapper script to the image
COPY Docker/run.sh /opt/sd/run.sh

VOLUME /opt/sd
VOLUME /hab

# Set Entrypoint
ENTRYPOINT ["/opt/sd/tini", "--", "/opt/sd/launch"]
