#!/usr/bin/env bash
# Run only on an isolated CI runner, with packages built by this checkout.
set -euo pipefail
version=$(python3 scripts/version.py)
deb_version=$(python3 scripts/version.py --format debian)
upgrade_version="$deb_version.1"
package="dist/open-download-manager_${version}_amd64.deb"
sudo dpkg --install "$package"
trap 'sudo dpkg --remove open-download-manager' EXIT
test "$(/usr/bin/odm --version)" = "$version"
test -f /usr/share/doc/open-download-manager/THIRD-PARTY-GO-NOTICES.txt
python3 scripts/smoke.py --exe /usr/bin/odm
python3 scripts/native_mode_test.py --exe /usr/bin/odm
# Reinstall the same artifact, then test a package-version upgrade in a fixture.
sudo dpkg --install "$package"
test "$(/usr/bin/odm --version)" = "$version"
upgrade_root=$(mktemp -d)
dpkg-deb --raw-extract "$package" "$upgrade_root/root"
sed -i "s/^Version: .*/Version: $upgrade_version/" "$upgrade_root/root/DEBIAN/control"
dpkg-deb --root-owner-group --build "$upgrade_root/root" "$upgrade_root/upgrade.deb"
sudo dpkg --install "$upgrade_root/upgrade.deb"
test "$(dpkg-query -W -f='${Version}' open-download-manager)" = "$upgrade_version"
python3 scripts/native_mode_test.py --exe /usr/bin/odm
sudo dpkg --remove open-download-manager
trap - EXIT
test ! -e /usr/bin/odm
echo 'PASS: Debian x64 install, real transfer, reinstall, package-version upgrade and uninstall'
