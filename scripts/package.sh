#!/usr/bin/env bash
set -Eeuo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
VERSION="0.1.0"
OUT="$ROOT/dist"
while (($#)); do
  case "$1" in
    --version) VERSION="$2"; shift 2 ;;
    *) printf 'unknown option: %s\n' "$1" >&2; exit 2 ;;
  esac
done
[[ "$VERSION" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] || { printf 'version must be x.y.z\n' >&2; exit 2; }
rm -rf "$OUT" "$ROOT/.package-work"
mkdir -p "$OUT" "$ROOT/.package-work/deb/DEBIAN" "$ROOT/.package-work/deb/usr/bin" "$ROOT/.package-work/deb/usr/share/lws"
install -m 0755 "$ROOT/scripts/lwsctl" "$ROOT/.package-work/deb/usr/bin/lwsctl"
install -m 0644 "$ROOT/infrastructure/compose.yaml" "$ROOT/.package-work/deb/usr/share/lws/compose.yaml"
cat >"$ROOT/.package-work/deb/DEBIAN/control" <<EOF
Package: lws
Version: $VERSION
Section: admin
Priority: optional
Architecture: amd64
Maintainer: LabWebSystem maintainers
Description: LabWebSystem lifecycle tools
EOF
install -m 0755 "$ROOT/packaging/lws.prerm" "$ROOT/.package-work/deb/DEBIAN/prerm"
if command -v dpkg-deb >/dev/null 2>&1; then
  dpkg-deb --build "$ROOT/.package-work/deb" "$OUT/lws_${VERSION}_amd64.deb" >/dev/null
else
  command -v ar >/dev/null 2>&1 || { printf 'dpkg-deb or ar is required\n' >&2; exit 1; }
  (cd "$ROOT/.package-work/deb/DEBIAN" && tar -czf "$ROOT/.package-work/control.tar.gz" .)
  (cd "$ROOT/.package-work/deb" && tar -czf "$ROOT/.package-work/data.tar.gz" --exclude=DEBIAN .)
  printf '2.0\n' >"$ROOT/.package-work/debian-binary"
  ar r "$OUT/lws_${VERSION}_amd64.deb" "$ROOT/.package-work/debian-binary" "$ROOT/.package-work/control.tar.gz" "$ROOT/.package-work/data.tar.gz" >/dev/null
fi
command -v rpmbuild >/dev/null 2>&1 || { printf 'rpmbuild is required\n' >&2; exit 1; }
RPMROOT="$ROOT/.package-work/rpm"
mkdir -p "$RPMROOT/BUILD" "$RPMROOT/RPMS" "$RPMROOT/SOURCES" "$RPMROOT/SPECS" "$RPMROOT/SRPMS"
cp "$ROOT/scripts/lwsctl" "$RPMROOT/SOURCES/lwsctl"
cp "$ROOT/infrastructure/compose.yaml" "$RPMROOT/SOURCES/compose.yaml"
sed "s/^Version: VERSION$/Version: $VERSION/" "$ROOT/packaging/lws.spec.in" >"$RPMROOT/SPECS/lws.spec"
rpmbuild --define "_topdir $RPMROOT" -bb "$RPMROOT/SPECS/lws.spec" >/dev/null
cp "$RPMROOT"/RPMS/*/lws-"$VERSION"-1.*.rpm "$OUT/lws-${VERSION}.rpm"
sha256sum "$OUT"/* >"$OUT/SHA256SUMS"
rm -rf "$ROOT/.package-work"
printf 'Built release artifacts in %s\n' "$OUT"
