#!/usr/bin/env bash
set -Eeuo pipefail

if [[ ${EUID:-$(id -u)} -ne 0 ]]; then
  echo "请使用 sudo 或 root 运行 install.sh" >&2
  exit 1
fi

root_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
[[ -x "$root_dir/frp-manager" ]] || { echo "发行包缺少 frp-manager" >&2; exit 1; }
[[ -x "$root_dir/frps" ]] || { echo "发行包缺少 frps" >&2; exit 1; }
[[ -d "$root_dir/web" ]] || { echo "发行包缺少 web 目录" >&2; exit 1; }

command -v systemctl >/dev/null || { echo "当前系统不支持 systemd" >&2; exit 1; }
command -v nft >/dev/null || { echo "缺少 nftables，请先安装" >&2; exit 1; }
command -v openssl >/dev/null || { echo "缺少 openssl，请先安装" >&2; exit 1; }
command -v ss >/dev/null || { echo "缺少 ss，请先安装 iproute2" >&2; exit 1; }

if ! getent group frp >/dev/null; then
  groupadd --system frp
fi
if ! id frp >/dev/null 2>&1; then
  useradd --system --gid frp --home-dir /nonexistent --shell /usr/sbin/nologin frp
fi

install -m 755 "$root_dir/frp-manager" /usr/local/bin/frp-manager
install -m 755 "$root_dir/frps" /usr/local/bin/frps
install -d -m 755 /opt/frp-manager/web
cp -a "$root_dir/web/." /opt/frp-manager/web/
find /opt/frp-manager/web -type d -exec chmod 755 {} +
find /opt/frp-manager/web -type f -exec chmod 644 {} +

install -m 644 "$root_dir/deploy/frps.service" /etc/systemd/system/frps.service
install -m 644 "$root_dir/deploy/frp-control.service" /etc/systemd/system/frp-control.service
install -m 644 "$root_dir/deploy/maplink-server.service" /etc/systemd/system/maplink-server.service
install -d -m 755 /usr/local/lib/maplink-server
install -m 755 "$root_dir/scripts/bootstrap.sh" /usr/local/lib/maplink-server/bootstrap.sh

if [[ ! -s /etc/frp-manager/state.json || ! -s /etc/frp-manager/admin-password.hash ]]; then
  /usr/local/lib/maplink-server/bootstrap.sh "$@"
else
  systemctl daemon-reload
  systemctl enable frps.service frp-control.service maplink-server.service
  systemctl restart frps.service
  systemctl restart frp-control.service
  systemctl restart maplink-server.service
  echo "MapLink Server 已升级并重启，现有配置和证书保持不变。"
fi

/usr/local/bin/frp-manager version
/usr/local/bin/frps --version
