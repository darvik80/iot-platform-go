#!/usr/bin/env bash
# Build .deb package for iot-platform-go
# Usage: ./deb/build.sh [version]
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
cd "$PROJECT_ROOT"

# ── Version ──────────────────────────────────────────────────────────
if [ -n "${1:-}" ]; then
    VERSION="$1"
elif [ -f "$PROJECT_ROOT/version.txt" ]; then
    VERSION="$(tr -d '[:space:]' < "$PROJECT_ROOT/version.txt")"
else
    VERSION="0.0.0-$(date +%Y%m%d)"
fi
echo "==> Building iot-platform-go_${VERSION}_amd64.deb"

# ── Build web UI ─────────────────────────────────────────────────────
echo "==> Building web UI"
cd web && npm ci && npm run build
cd "$PROJECT_ROOT"

# ── Build Linux binary ───────────────────────────────────────────────
echo "==> Building Linux binary"
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o bin/platform ./cmd/platform

# ── Prepare staging area ─────────────────────────────────────────────
STAGE=$(mktemp -d)
trap 'rm -rf "$STAGE"' EXIT

PKG="${STAGE}/iot-platform-go"
mkdir -p "${PKG}/DEBIAN"
mkdir -p "${PKG}/usr/bin"
mkdir -p "${PKG}/etc/iot-platform-go"
mkdir -p "${PKG}/lib/systemd/system"
mkdir -p "${PKG}/var/lib/iot-platform-go/data"

# Binary
install -m 0755 bin/platform "${PKG}/usr/bin/iot-platform"

# Default config (empty → triggers setup wizard on first boot)
cat > "${PKG}/etc/iot-platform-go/config.yml" <<'EOF'
# IoT Platform Go — configuration file
# Leave empty to run the web setup wizard on first boot.
# The wizard is available at http://<host>:8080 after the service starts.
EOF
chmod 0644 "${PKG}/etc/iot-platform-go/config.yml"

# Systemd unit
install -m 0644 "${SCRIPT_DIR}/iot-platform.service" "${PKG}/lib/systemd/system/iot-platform.service"

# Data directory ownership
chmod 0755 "${PKG}/var/lib/iot-platform-go"
chmod 0755 "${PKG}/var/lib/iot-platform-go/data"

# ── DEBIAN control files ─────────────────────────────────────────────
sed "s/^Version:.*/Version: ${VERSION}/" "${SCRIPT_DIR}/DEBIAN/control" \
    > "${PKG}/DEBIAN/control"

for f in preinst postinst prerm postrm; do
    install -m 0755 "${SCRIPT_DIR}/DEBIAN/${f}" "${PKG}/DEBIAN/${f}"
done

# ── Build .deb ───────────────────────────────────────────────────────
mkdir -p dist
echo "==> Running dpkg-deb"
dpkg-deb --build --root-owner-group "${PKG}" "dist/iot-platform-go_${VERSION}_amd64.deb"

echo ""
echo "==> Package ready: dist/iot-platform-go_${VERSION}_amd64.deb"
ls -lh "dist/iot-platform-go_${VERSION}_amd64.deb"
echo ""
echo "Install with:"
echo "  sudo dpkg -i dist/iot-platform-go_${VERSION}_amd64.deb"
echo "  sudo apt-get install -f   # fix dependencies if needed"
