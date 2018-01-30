pkg_name=launcher
pkg_origin=screwdriver-cd
pkg_version=`git describe --abbrev=0 --tags`
pkg_scaffolding=core/scaffolding-go
pkg_license=('BSD 3-clause')
pkg_maintainer=('St. John Johnson <st.john.johnson@gmail.com>')
pkg_deps=(core/bash)
pkg_bin_dirs=(bin)

scaffolding_go_base_path="github.com/screwdriver-cd"
scaffolding_go_build_deps=(
    gopkg.in/kr/pty.v1
    gopkg.in/myesui/uuid.v1
    gopkg.in/urfave/cli.v1
)

do_install() {
    export VERSION="${pkg_version}"
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
