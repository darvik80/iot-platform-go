#!/usr/bin/env bash
# Build .deb package for iot-platform
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
echo "==> Building iot-platform_${VERSION}_amd64.deb"

# ── Build web UI ─────────────────────────────────────────────────────
echo "==> Building web UI"
cd web && npm ci && npm run build
cd "$PROJECT_ROOT"

# ── Build Linux binary ───────────────────────────────────────────────
echo "==> Building Linux binary"
COMMIT="$(git rev-parse --short HEAD 2>/dev/null || echo unknown)"
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w -X main.Version=${VERSION} -X main.Commit=${COMMIT}" -o bin/platform ./cmd/platform

# ── Prepare staging area ─────────────────────────────────────────────
# Layout:
#   /opt/iot-platform/{bin,etc,lib} — application (certs go under etc/)
#   /var/log/iot-platform           — logs
#   /etc/logrotate.d/iot-platform   — log rotation
STAGE=$(mktemp -d)
trap 'rm -rf "$STAGE"' EXIT

PKG="${STAGE}/iot-platform"
mkdir -p "${PKG}/DEBIAN"
mkdir -p "${PKG}/opt/iot-platform/bin"
mkdir -p "${PKG}/opt/iot-platform/etc/certs"
mkdir -p "${PKG}/opt/iot-platform/lib"
mkdir -p "${PKG}/var/log/iot-platform"
mkdir -p "${PKG}/etc/logrotate.d"
mkdir -p "${PKG}/lib/systemd/system"

# Binary
install -m 0755 bin/platform "${PKG}/opt/iot-platform/bin/platform"

# Default config (empty → triggers setup wizard on first boot)
cat > "${PKG}/opt/iot-platform/etc/config.yml" <<'EOF'
# IoT Platform Go — configuration file
# Leave empty to run the web setup wizard on first boot.
# The wizard is available at http://<host>:8080 after the service starts.
EOF
chmod 0644 "${PKG}/opt/iot-platform/etc/config.yml"

# Log rotation
install -m 0644 "${SCRIPT_DIR}/logrotate" "${PKG}/etc/logrotate.d/iot-platform"

# Systemd unit
install -m 0644 "${SCRIPT_DIR}/iot-platform.service" "${PKG}/lib/systemd/system/iot-platform.service"

# Directory ownership
chmod 0755 "${PKG}/opt/iot-platform"
chmod 0755 "${PKG}/opt/iot-platform/etc"
chmod 0755 "${PKG}/opt/iot-platform/etc/certs"
chmod 0755 "${PKG}/opt/iot-platform/lib"
chmod 0755 "${PKG}/var/log/iot-platform"

# ── DEBIAN control files ─────────────────────────────────────────────
sed "s/^Version:.*/Version: ${VERSION}/" "${SCRIPT_DIR}/DEBIAN/control" \
    > "${PKG}/DEBIAN/control"

for f in preinst postinst prerm postrm; do
    install -m 0755 "${SCRIPT_DIR}/DEBIAN/${f}" "${PKG}/DEBIAN/${f}"
done

# ── Build .deb ───────────────────────────────────────────────────────
mkdir -p dist
echo "==> Running dpkg-deb"
dpkg-deb --build --root-owner-group "${PKG}" "dist/iot-platform_${VERSION}_amd64.deb"

echo ""
echo "==> Package ready: dist/iot-platform_${VERSION}_amd64.deb"
ls -lh "dist/iot-platform_${VERSION}_amd64.deb"
echo ""
echo "Install with:"
echo "  sudo dpkg -i dist/iot-platform_${VERSION}_amd64.deb"
echo "  sudo apt-get install -f   # fix dependencies if needed"
