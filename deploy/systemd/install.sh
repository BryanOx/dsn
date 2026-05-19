#!/bin/bash
set -euo pipefail

if [ "$EUID" -ne 0 ]; then
    echo "Please run as root"
    exit 1
fi

echo "Creating dsn user..."
id -u dsn &>/dev/null || useradd --system --no-create-home --shell /usr/sbin/nologin dsn

echo "Creating directories..."
mkdir -p /var/lib/dsn /etc/dsn
chown -R dsn:dsn /var/lib/dsn

echo "Installing systemd unit..."
cp dsn.service /etc/systemd/system/
systemctl daemon-reload
systemctl enable dsn

echo "Done. Configure /etc/dsn/config.env then: systemctl start dsn"