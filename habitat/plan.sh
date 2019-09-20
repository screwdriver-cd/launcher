pkg_name=launcher
pkg_origin=screwdriver-cd
pkg_scaffolding=core/scaffolding-go
pkg_license=('BSD 3-clause')
pkg_maintainer=('St. John Johnson <st.john.johnson@gmail.com>')
pkg_upstream_url=("https://github.com/${pkg_origin}/${pkg_name}")
pkg_deps=(core/bash)
pkg_build_deps=(
    core/curl
    core/grep
    core/sed
)
pkg_bin_dirs=(bin)

# Scaffolding based on https://github.com/habitat-sh/core-plans/tree/master/scaffolding-go
scaffolding_go_base_path="github.com/screwdriver-cd"
scaffolding_go_build_deps=(
    github.com/creack/pty
    github.com/urfave/cli
    gopkg.in/myesui/uuid.v1
    gopkg.in/fatih/color.v1
)

# Extract the version from the last published GitHub release
pkg_version() {
    $(pkg_path_for core/curl)/bin/curl -I \
        https://github.com/${pkg_origin}/${pkg_name}/releases/latest | \
        $(pkg_path_for core/grep)/bin/grep Location | \
        $(pkg_path_for core/sed)/bin/sed -E 's#.*/tag/v(.*)$#\1#' | \
        $(pkg_path_for core/sed)/bin/sed 's/[^0-9.]*//g'
}

do_before() {
    do_default_before
    update_pkg_version
}

do_install() {
    export VERSION="$(pkg_version)"
    export DATE=`date -u '+%Y-%m-%dT%T.00Z'`

    pushd "$scaffolding_go_pkg_path"
    go install -ldflags "-X main.version=${VERSION} -X main.date=${DATE}"
    popd
    cp -r "${scaffolding_go_gopath:?}/bin" "${pkg_prefix}/${bin}"

    wrap_bin "${pkg_prefix}/bin/launcher"
}

wrap_bin() {
    local bin="$1"
    build_line "Adding wrapper $bin to ${bin}.real"
    mv -v "$bin" "${bin}.real"
    cat <<EOF > "${bin}"
#!$(pkg_path_for bash)/bin/bash
set -e
if test -n "$DEBUG"; then set -x; fi
export SD_SHELL_BIN="$(pkg_path_for bash)/bin/bash"

exec ${bin}.real \$@
EOF
    chmod -v 755 "$bin"
}
