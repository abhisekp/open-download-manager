#!/usr/bin/env bash
# Run only on an isolated CI runner, with packages built by this checkout.
set -euo pipefail
package="dist/open-download-manager_0.1.0-dev_amd64.deb"
sudo dpkg --install "$package"
trap 'sudo dpkg --remove open-download-manager' EXIT
test "$(/usr/bin/odm --version)" = 0.1.0-dev
test -f /usr/share/doc/open-download-manager/THIRD-PARTY-GO-NOTICES.txt
python3 scripts/smoke.py --exe /usr/bin/odm
python3 scripts/native_mode_test.py --exe /usr/bin/odm
# Reinstallation checks package replacement; it does not claim a version upgrade.
sudo dpkg --install "$package"
test "$(/usr/bin/odm --version)" = 0.1.0-dev
sudo dpkg --remove open-download-manager
trap - EXIT
test ! -e /usr/bin/odm
echo 'PASS: Debian x64 install, real transfer, reinstall and uninstall'
