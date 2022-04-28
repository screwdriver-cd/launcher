FROM alpine:3.12 AS base
LABEL MAINTAINER="Screwdriver Team <screwdriver.cd>"

ARG TARGETOS TARGETARCH
RUN echo "Building for ${TARGETOS}_${TARGETARCH}"

WORKDIR /opt/sd

FROM base AS base-amd64
RUN set -x \
   # Alpine ships with musl instead of glibc (this fixes the symlink)
   && mkdir /lib64 \
   && ln -s /lib/libc.musl-x86_64.so.1 /lib64/ld-linux-x86-64.so.2 \
   # Also, missing https for some magic reason
   && apk add --no-cache --update ca-certificates \
   && apk add --no-cache --virtual .build-dependencies wget gpgme unzip \
   # Install Habitat
   && mkdir -p /hab/bin /opt/sd/bin \
   # Download Habitat Binary
   && wget -O hab.tar.gz "https://packages.chef.io/files/stable/habitat/0.79.1/hab-x86_64-linux.tar.gz" \
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
   # Cleanup Habitat Files
   && rm -rf /hab/cache /opt/sd/hab.tar.gz /opt/sd/hab-* \
   # Cleanup docs and man pages (how could this go wrong)
   && find /hab -name doc -exec rm -r {} + \
   && find /hab -name docs -exec rm -r {} + \
   && find /hab -name man -exec rm -r {} + \
   # bin link bash if not present
   && if [[ -z $(command -v bash) ]]; then /hab/bin/hab pkg binlink core/bash bash ; fi

FROM base AS base-arm64
RUN set -x \
   # Alpine ships with musl instead of glibc (this fixes the symlink)
   && mkdir /lib64 \
   && ln -s /lib/libc.musl-aarch64.so.1 /lib64/ld-linux-aarch64.so.1 \
   && apk add --no-cache --update ca-certificates \
   && apk add --no-cache --virtual .build-dependencies gpgme \
   # Donwload pkgs needed in container
   && apk add --no-cache composer wget zip unzip git bash iptables sed docker jq curl kmod

# Install common dependencies by target architcture
FROM base-${TARGETARCH} AS final
RUN set -x \
   # Download Launcher
   && wget -q -O - https://github.com/screwdriver-cd/launcher/releases/latest \
   | egrep -o "/screwdriver-cd/launcher/releases/download/v[0-9.]*/launcher_${TARGETOS}_${TARGETARCH}" \
   | sed -e "s/\/screwdriver-cd\/\([a-zA-Z-]*\)\/releases\/download\/\(v[0-9.]*\)\/launcher_${TARGETOS}_${TARGETARCH}/\1 \2/" >> tool-versions \
   && wget -q -O - https://github.com/screwdriver-cd/launcher/releases/latest \
   | egrep -o "/screwdriver-cd/launcher/releases/download/v[0-9.]*/launcher_${TARGETOS}_${TARGETARCH}" \
   | wget --base=http://github.com/ -i - -O launch \
   && chmod +x launch \
   # Download Log Service
   && wget -q -O - https://github.com/screwdriver-cd/log-service/releases/latest \
   | egrep -o "/screwdriver-cd/log-service/releases/download/v[0-9.]*/log-service_${TARGETOS}_${TARGETARCH}" \
   | sed -e "s/\/screwdriver-cd\/\([a-zA-Z-]*\)\/releases\/download\/\(v[0-9.]*\)\/log-service_${TARGETOS}_${TARGETARCH}/\1 \2/" >> tool-versions \
   && wget -q -O - https://github.com/screwdriver-cd/log-service/releases/latest \
   | egrep -o "/screwdriver-cd/log-service/releases/download/v[0-9.]*/log-service_${TARGETOS}_${TARGETARCH}" \
   | wget --base=http://github.com/ -i - -O logservice \
   && chmod +x logservice \
   # Download Meta CLI
   && wget -q -O - https://github.com/screwdriver-cd/meta-cli/releases/latest \
   | egrep -o "/screwdriver-cd/meta-cli/releases/download/v[0-9.]*/meta-cli_${TARGETOS}_${TARGETARCH}" \
   | sed -e "s/\/screwdriver-cd\/\([a-zA-Z-]*\)\/releases\/download\/\(v[0-9.]*\)\/meta-cli_${TARGETOS}_${TARGETARCH}/\1 \2/" >> tool-versions \
   && wget -q -O - https://github.com/screwdriver-cd/meta-cli/releases/latest \
   | egrep -o "/screwdriver-cd/meta-cli/releases/download/v[0-9.]*/meta-cli_${TARGETOS}_${TARGETARCH}" \
   | wget --base=http://github.com/ -i - -O meta \
   && chmod +x meta \
   # Download sd-step
   && wget -q -O - https://github.com/screwdriver-cd/sd-step/releases/latest \
   | egrep -o "/screwdriver-cd/sd-step/releases/download/v[0-9.]*/sd-step_${TARGETOS}_${TARGETARCH}" \
   | sed -e "s/\/screwdriver-cd\/\([a-zA-Z-]*\)\/releases\/download\/\(v[0-9.]*\)\/sd-step_${TARGETOS}_${TARGETARCH}/\1 \2/" >> tool-versions \
   && wget -q -O - https://github.com/screwdriver-cd/sd-step/releases/latest \
   | egrep -o "/screwdriver-cd/sd-step/releases/download/v[0-9.]*/sd-step_${TARGETOS}_${TARGETARCH}" \
   | wget --base=http://github.com/ -i - -O sd-step \
   && chmod +x sd-step \
   # Download sd-cmd
   && wget -q -O - https://github.com/screwdriver-cd/sd-cmd/releases/latest \
   | egrep -o "/screwdriver-cd/sd-cmd/releases/download/v[0-9.]*/sd-cmd_${TARGETOS}_${TARGETARCH}" \
   | sed -e "s/\/screwdriver-cd\/\([a-zA-Z-]*\)\/releases\/download\/\(v[0-9.]*\)\/sd-cmd_${TARGETOS}_${TARGETARCH}/\1 \2/" >> tool-versions \
   && wget -q -O - https://github.com/screwdriver-cd/sd-cmd/releases/latest \
   | egrep -o "/screwdriver-cd/sd-cmd/releases/download/v[0-9.]*/sd-cmd_${TARGETOS}_${TARGETARCH}" \
   | wget --base=http://github.com/ -i - -O sd-cmd \
   && chmod +x sd-cmd \
   # Download store-cli
   && wget -q -O - https://github.com/screwdriver-cd/store-cli/releases/latest \
   | egrep -o "/screwdriver-cd/store-cli/releases/download/v[0-9.]*/store-cli_${TARGETOS}_${TARGETARCH}" \
   | sed -e "s/\/screwdriver-cd\/\([a-zA-Z-]*\)\/releases\/download\/\(v[0-9.]*\)\/store-cli_${TARGETOS}_${TARGETARCH}/\1 \2/" >> tool-versions \
   && wget -q -O - https://github.com/screwdriver-cd/store-cli/releases/latest \
   | egrep -o "/screwdriver-cd/store-cli/releases/download/v[0-9.]*/store-cli_${TARGETOS}_${TARGETARCH}" \
   | wget --base=http://github.com/ -i - -O store-cli \
   && chmod +x store-cli \
   # Download gitversion
   && wget -q -O - https://github.com/screwdriver-cd/gitversion/releases/latest \
   | egrep -o "/screwdriver-cd/gitversion/releases/download/v[0-9.]*/gitversion_${TARGETOS}_${TARGETARCH}" \
   | sed -e "s/\/screwdriver-cd\/\([a-zA-Z-]*\)\/releases\/download\/\(v[0-9.]*\)\/gitversion_${TARGETOS}_${TARGETARCH}/\1 \2/" >> tool-versions \
   && wget -q -O - https://github.com/screwdriver-cd/gitversion/releases/latest \
   | egrep -o "/screwdriver-cd/gitversion/releases/download/v[0-9.]*/gitversion_${TARGETOS}_${TARGETARCH}" \
   | wget --base=http://github.com/ -i - -O gitversion \
   && chmod +x gitversion \
   # Download Tini Static
   && wget -q -O - https://github.com/krallin/tini/releases/latest \
   | egrep -o "/krallin/tini/releases/download/v[0-9.]*/tini-static" \
   | head -1 \
   | wget --base=http://github.com/ -i - -O tini-static \
   && wget -q -O - https://github.com/krallin/tini/releases/latest \
   | egrep -o "/krallin/tini/releases/download/v[0-9.]*/tini-static.asc" \
   | wget --base=http://github.com/ -i - -O tini-static.asc \
   && found=''; \
   ( \
   gpg --no-tty --keyserver keyserver.ubuntu.com --recv-keys 595E85A6B1B4779EA4DAAEC70B588DFF0527A9B7 || \
   gpg --no-tty --keyserver pgp.mit.edu --recv-keys 595E85A6B1B4779EA4DAAEC70B588DFF0527A9B7 || \
   gpg --no-tty --keyserver hkp://p80.pool.sks-keyservers.net:80 --recv-keys 595E85A6B1B4779EA4DAAEC70B588DFF0527A9B7 || \
   gpg --no-tty --keyserver keyserver.pgp.com --recv-keys 595E85A6B1B4779EA4DAAEC70B588DFF0527A9B7 || \
   gpg --no-tty --keyserver hkp://ipv4.pool.sks-keyservers.net --recv-keys 595E85A6B1B4779EA4DAAEC70B588DFF0527A9B7 || \
   gpg --no-tty --keyserver ha.pool.sks-keyservers.net --recv-keys 595E85A6B1B4779EA4DAAEC70B588DFF0527A9B7 \
   ) \
   && found=yes && break; \
   test -z "$found" && echo >&2 "error: failed to fetch GPG key 595E85A6B1B4779EA4DAAEC70B588DFF0527A9B7" && exit 1; \
   gpg --verify tini-static.asc  \
   && rm tini-static.asc \
   && mv tini-static tini \
   && chmod +x tini \
   # Download dumb-init
   && wget -O /usr/local/bin/dumb-init "https://github.com/Yelp/dumb-init/releases/download/v1.2.2/dumb-init_1.2.2_${TARGETARCH}" \
   && chmod +x /usr/local/bin/dumb-init \
   && cp /usr/local/bin/dumb-init /opt/sd/dumb-init \
   # Install Sonar scanner cli
   && wget -O sonarscanner-cli-linux.zip "https://binaries.sonarsource.com/Distribution/sonar-scanner-cli/sonar-scanner-cli-4.6.2.2472-linux.zip" \
   && wget -O sonarscanner-cli-macosx.zip "https://binaries.sonarsource.com/Distribution/sonar-scanner-cli/sonar-scanner-cli-4.6.2.2472-macosx.zip" \
   && unzip -q sonarscanner-cli-linux.zip \
   && unzip -q sonarscanner-cli-macosx.zip \
   && mv sonar-scanner-*-linux sonarscanner-cli-linux \
   && mv sonar-scanner-*-macosx sonarscanner-cli-macosx \
   # Install skope
   && wget -q -O skopeo-linux.tar.gz "https://github.com/screwdriver-cd/sd-packages/releases/download/v0.0.30/skopeo-linux.tar.gz" \
   && tar -C . -ozxvf skopeo-linux.tar.gz \
   && chmod +x skopeo \
   # Install zstd
   && wget -q -O zstd-cli-linux.tar.gz "https://github.com/screwdriver-cd/sd-packages/releases/download/v0.0.30/zstd-cli-linux.tar.gz" \
   && wget -q -O zstd-cli-macosx.tar.gz "https://github.com/screwdriver-cd/sd-packages/releases/download/v0.0.30/zstd-cli-macosx.tar.gz" \
   && tar -C . -ozxvf zstd-cli-linux.tar.gz \
   && mv zstd zstd-cli-linux \
   && tar -C . -ozxvf zstd-cli-macosx.tar.gz \
   && mv zstd zstd-cli-macosx \
   && chmod +x zstd-cli-linux \
   && chmod +x zstd-cli-macosx \
   # Cleanup Skopeo and Sonar scanner cli files
   && rm -rf /opt/sd/skopeo-linux.tar.gz /opt/sd/sonarscanner-cli-linux.zip /opt/sd/sonarscanner-cli-macosx.zip /opt/sd/sonar-scanner-*-linux /opt/sd/sonar-scanner-*-macosx \
   # Cleanup Zstd cli files
   && rm -rf /opt/sd/zstd-cli-linux.tar.gz /opt/sd/zstd-cli-macosx.tar.gz \
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
ENTRYPOINT ["/opt/sd/launcher_entrypoint.sh"]
