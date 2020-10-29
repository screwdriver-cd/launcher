FROM alpine:3.12
LABEL MAINTAINER="Screwdriver Team <screwdriver.cd>"

WORKDIR /opt/sd
RUN set -x \
   # Alpine ships with musl instead of glibc (this fixes the symlink)
   && mkdir /lib64 \
   && ln -s /lib/libc.musl-x86_64.so.1 /lib64/ld-linux-x86-64.so.2 \
   # Also, missing https for some magic reason
   && apk add --no-cache --update ca-certificates \
   && apk add --no-cache --virtual .build-dependencies wget gpgme unzip \
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
   # Download store-cli
   && wget -q -O - https://github.com/screwdriver-cd/store-cli/releases/latest \
      | egrep -o '/screwdriver-cd/store-cli/releases/download/v[0-9.]*/store-cli_linux_amd64' \
      | wget --base=http://github.com/ -i - -O store-cli \
   && chmod +x store-cli \
   # Download dumb-init
   && wget -O /usr/local/bin/dumb-init https://github.com/Yelp/dumb-init/releases/download/v1.2.2/dumb-init_1.2.2_amd64 \
   && chmod +x /usr/local/bin/dumb-init \
   && cp /usr/local/bin/dumb-init /opt/sd/dumb-init \
   # Install Habitat
   && mkdir -p /hab/bin /opt/sd/bin \
   # Download Habitat Binary
   && wget -O hab.tar.gz 'https://api.bintray.com/content/habitat/stable/linux/x86_64/hab-0.79.1-20190410220617-x86_64-linux.tar.gz?bt_package=hab-x86_64-linux' \
   && tar -C . -ozxvf hab.tar.gz \
   && mv hab-*/hab /hab/bin/hab \
   && chmod +x /hab/bin/hab \
   # @TODO Remove this, I don't think it belongs here.  We should use /hab/bin/hab instead.
   && cp /hab/bin/hab /opt/sd/bin/hab \
   # Install Habitat packages
   && /hab/bin/hab pkg install core/bash core/git core/zip core/unzip core/kmod core/iptables core/docker/19.03.8 core/wget core/sed core/jq-static/1.6 \
   # Install curl 7.54.1 since we use that version in artifact-bookend
   # https://github.com/screwdriver-cd/artifact-bookend/blob/master/commands.txt
   && /hab/bin/hab pkg install core/curl/7.54.1 \
   # Install Sonar scanner cli
   && wget -O sonarscanner-cli-linux.zip 'https://binaries.sonarsource.com/Distribution/sonar-scanner-cli/sonar-scanner-cli-4.4.0.2170-linux.zip' \
   && wget -O sonarscanner-cli-macosx.zip 'https://binaries.sonarsource.com/Distribution/sonar-scanner-cli/sonar-scanner-cli-4.4.0.2170-macosx.zip' \
   && unzip -q sonarscanner-cli-linux.zip \
   && unzip -q sonarscanner-cli-macosx.zip \
   && mv sonar-scanner-*-linux sonarscanner-cli-linux \
   && mv sonar-scanner-*-macosx sonarscanner-cli-macosx \
   # Cleanup Habitat Files
   && rm -rf /hab/cache /opt/sd/hab.tar.gz /opt/sd/hab-* \
   # Cleanup docs and man pages (how could this go wrong)
   && find /hab -name doc -exec rm -r {} + \
   && find /hab -name docs -exec rm -r {} + \
   && find /hab -name man -exec rm -r {} + \
   # Cleanup Sonar scanner cli files
   && rm -rf /opt/sd/sonarscanner-cli-linux.zip /opt/sd/sonarscanner-cli-macosx.zip /opt/sd/sonar-scanner-*-linux /opt/sd/sonar-scanner-*-macosx \
   # Cleanup packages
   && apk del --purge .build-dependencies

# Copy optional entrypoint script to the image
COPY Docker/launcher_entrypoint.sh /opt/sd/launcher_entrypoint.sh

# Copy wrapper script to the image
COPY Docker/run.sh /opt/sd/run.sh
COPY Docker/local_run.sh /opt/sd/local_run.sh

VOLUME /opt/sd
VOLUME /hab

# Set Entrypoint
ENTRYPOINT ["/opt/sd/dumb-init", "--"]

CMD ["/opt/sd/launcher_entrypoint.sh", ""]
