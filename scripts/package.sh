#!/usr/bin/env bash
set -Eeuo pipefail

readonly ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
readonly OUT="$ROOT/dist"
readonly WORK="$ROOT/.package-work"

die() {
  printf '%s\n' "$*" >&2
  exit 1
}

cleanup() {
  rm -rf "$WORK"
}

prepare() {
  rm -rf "$OUT" "$WORK"

  mkdir -p \
    "$OUT" \
    "$WORK/deb/DEBIAN" \
    "$WORK/deb/usr/bin" \
    "$WORK/deb/usr/share/lws"
}

render_compose() {
  local backend_default='sha256:c1c71a7693cd3981884ecc669ab72934dda8e9fbb2b15125ff1e212f9f82eda2'
  local dashboard_default='sha256:be36c3a68689ddbf480a801e8752b38ef7a978774a97bbb881d0fa70432047c4'
  local backend_digest="${LWS_BACKEND_DIGEST:-$backend_default}"
  local dashboard_digest="${LWS_DASHBOARD_DIGEST:-$dashboard_default}"

  sed \
    -e "s|\\\${LWS_BACKEND_DIGEST:-$backend_default}|$backend_digest|g" \
    -e "s|\\\${LWS_DASHBOARD_DIGEST:-$dashboard_default}|$dashboard_digest|g" \
    "$ROOT/infrastructure/compose.yaml" \
    >"$WORK/compose.yaml"
}

build_lwsctl() {
  "$ROOT/scripts/build-lwsctl.sh" \
    --output "$WORK/lwsctl" \
    --goos linux \
    --goarch amd64
}

prepare_deb_tree() {
  install -m 0755 \
    "$WORK/lwsctl" \
    "$WORK/deb/usr/bin/lwsctl"

  install -m 0644 \
    "$WORK/compose.yaml" \
    "$WORK/deb/usr/share/lws/compose.yaml"

  install -m 0644 \
    "$ROOT/infrastructure/Corefile" \
    "$WORK/deb/usr/share/lws/Corefile"

  install -m 0755 \
    "$ROOT/scripts/install.sh" \
    "$WORK/deb/usr/share/lws/install.sh"

  install -m 0755 \
    "$ROOT/packaging/lws.prerm" \
    "$WORK/deb/DEBIAN/prerm"

  install -m 0755 \
    "$ROOT/packaging/lws.preinst" \
    "$WORK/deb/DEBIAN/preinst"

  printf '%s\n' "$VERSION" \
    >"$WORK/deb/usr/share/lws/version"

  cat >"$WORK/deb/DEBIAN/control" <<EOF
Package: lws
Version: $VERSION
Section: admin
Priority: optional
Architecture: amd64
Maintainer: LabWebSystem maintainers
Description: LabWebSystemのライフサイクルツール
EOF
}

build_deb() {
  local output="$OUT/lws_${VERSION}_amd64.deb"

  if command -v dpkg-deb >/dev/null 2>&1; then
    dpkg-deb --build "$WORK/deb" "$output" >/dev/null
    return
  fi

  command -v ar >/dev/null 2>&1 ||
    die 'dpkg-debまたはarが必要です'

  (
    cd "$WORK/deb/DEBIAN"
    tar -czf "$WORK/control.tar.gz" .
  )

  (
    cd "$WORK/deb"
    tar -czf "$WORK/data.tar.gz" --exclude=DEBIAN .
  )

  printf '2.0\n' >"$WORK/debian-binary"

  ar r "$output" \
    "$WORK/debian-binary" \
    "$WORK/control.tar.gz" \
    "$WORK/data.tar.gz" \
    >/dev/null
}

build_rpm() {
  command -v rpmbuild >/dev/null 2>&1 ||
    die 'rpmbuildが必要です'

  local rpm_root="$WORK/rpm"

  mkdir -p \
    "$rpm_root/BUILD" \
    "$rpm_root/RPMS" \
    "$rpm_root/SOURCES" \
    "$rpm_root/SPECS" \
    "$rpm_root/SRPMS"

  install -m 0755 "$WORK/lwsctl" "$rpm_root/SOURCES/lwsctl"
  install -m 0644 "$WORK/compose.yaml" "$rpm_root/SOURCES/compose.yaml"
  install -m 0644 "$ROOT/infrastructure/Corefile" "$rpm_root/SOURCES/Corefile"
  install -m 0755 "$ROOT/scripts/install.sh" "$rpm_root/SOURCES/install.sh"

  printf '%s\n' "$VERSION" >"$rpm_root/SOURCES/version"

  sed \
    "s/^Version: VERSION$/Version: $VERSION/" \
    "$ROOT/packaging/lws.spec.in" \
    >"$rpm_root/SPECS/lws.spec"

  rpmbuild \
    --define "_topdir $rpm_root" \
    -bb "$rpm_root/SPECS/lws.spec" \
    >/dev/null

  cp \
    "$rpm_root"/RPMS/*/lws-"$VERSION"-1.*.rpm \
    "$OUT/lws-${VERSION}.x86_64.rpm"
}

write_checksums() {
  (
    cd "$OUT"
    sha256sum ./* >SHA256SUMS
  )
}

main() {
  (($# == 0)) || die 'パッケージのバージョンはmise run version coreで設定してください'

  VERSION="$("$ROOT/scripts/version.sh" core)"

  trap cleanup EXIT

  prepare
  render_compose
  build_lwsctl
  prepare_deb_tree
  build_deb
  build_rpm
  write_checksums

  printf 'リリース成果物を%sに生成しました\n' "$OUT"
}

main "$@"
