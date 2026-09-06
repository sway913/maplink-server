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

for dependency in systemctl nft openssl ss tar; do
  command -v "$dependency" >/dev/null || { echo "缺少系统依赖: $dependency" >&2; exit 1; }
done

if ! getent group frp >/dev/null; then
  groupadd --system frp
fi
if ! id frp >/dev/null 2>&1; then
  useradd --system --gid frp --home-dir /nonexistent --shell /usr/sbin/nologin frp
fi

initialized=false
if [[ -s /etc/frp-manager/state.json && -s /etc/frp-manager/admin-password.hash ]]; then
  initialized=true
fi

backup_dir=''
had_legacy_unit=false
had_current_unit=false
cutover_started=false

backup_existing_installation() {
  local stamp
  stamp=$(date -u +%Y%m%dT%H%M%SZ)
  backup_dir="/var/backups/maplink-server/$stamp"
  install -d -m 700 /var/backups/maplink-server "$backup_dir"
  tar -czf "$backup_dir/configs.tar.gz" -C / etc/frp-manager etc/frp
  if [[ -d /opt/frp-manager/web ]]; then
    tar -czf "$backup_dir/web.tar.gz" -C / opt/frp-manager/web
  fi
  for binary in frp-manager frps; do
    if [[ -f "/usr/local/bin/$binary" ]]; then
      cp "/usr/local/bin/$binary" "$backup_dir/$binary"
    fi
  done
  for unit in frp-manager.service maplink-server.service frps.service frp-control.service; do
    if [[ -f "/etc/systemd/system/$unit" ]]; then
      cp "/etc/systemd/system/$unit" "$backup_dir/$unit"
    fi
  done
  [[ -f /etc/systemd/system/frp-manager.service ]] && had_legacy_unit=true
  [[ -f /etc/systemd/system/maplink-server.service ]] && had_current_unit=true
}

restore_previous_installation() {
  local failed_status=$?
  trap - ERR
  if [[ "$cutover_started" == true && -n "$backup_dir" ]]; then
    echo "升级失败，正在恢复上一版本……" >&2
    systemctl disable --now maplink-server.service >/dev/null 2>&1 || true
    for binary in frp-manager frps; do
      [[ -f "$backup_dir/$binary" ]] && install -m 755 "$backup_dir/$binary" "/usr/local/bin/$binary"
    done
    [[ -f "$backup_dir/configs.tar.gz" ]] && tar -xzf "$backup_dir/configs.tar.gz" -C /
    if [[ -f "$backup_dir/web.tar.gz" ]]; then
      install -d -m 755 -o root -g root /opt/frp-manager/web
      find /opt/frp-manager/web -mindepth 1 -delete
      tar -xzf "$backup_dir/web.tar.gz" -C /
    fi
    for unit in maplink-server.service frps.service frp-control.service; do
      if [[ -f "$backup_dir/$unit" ]]; then
        install -m 644 "$backup_dir/$unit" "/etc/systemd/system/$unit"
      elif [[ "$unit" == maplink-server.service && "$had_current_unit" == false ]]; then
        rm -f "/etc/systemd/system/$unit"
      fi
    done
    if [[ "$had_legacy_unit" == true ]]; then
      install -m 644 "$backup_dir/frp-manager.service" /etc/systemd/system/frp-manager.service
    fi
    systemctl daemon-reload
    systemctl restart frps.service >/dev/null 2>&1 || true
    systemctl restart frp-control.service >/dev/null 2>&1 || true
    if [[ "$had_current_unit" == true ]]; then
      systemctl enable --now maplink-server.service >/dev/null 2>&1 || true
    elif [[ "$had_legacy_unit" == true ]]; then
      systemctl enable --now frp-manager.service >/dev/null 2>&1 || true
    fi
    echo "上一版本已恢复，备份位于 $backup_dir" >&2
  fi
  exit "$failed_status"
}

if [[ "$initialized" == true ]]; then
  backup_existing_installation
  trap restore_previous_installation ERR
  cutover_started=true
  systemctl disable --now frp-manager.service >/dev/null 2>&1 || true
  systemctl stop maplink-server.service >/dev/null 2>&1 || true
fi

install -m 755 "$root_dir/frp-manager" /usr/local/bin/frp-manager
install -m 755 "$root_dir/frps" /usr/local/bin/frps
install -d -m 755 -o root -g root /opt/frp-manager/web
find /opt/frp-manager/web -mindepth 1 -delete
cp -R "$root_dir/web/." /opt/frp-manager/web/
chown -R root:root /opt/frp-manager/web
find /opt/frp-manager/web -type d -exec chmod 755 {} +
find /opt/frp-manager/web -type f -exec chmod 644 {} +

install -m 644 "$root_dir/deploy/frps.service" /etc/systemd/system/frps.service
install -m 644 "$root_dir/deploy/frp-control.service" /etc/systemd/system/frp-control.service
install -m 644 "$root_dir/deploy/maplink-server.service" /etc/systemd/system/maplink-server.service
install -d -m 755 /usr/local/lib/maplink-server
install -m 755 "$root_dir/scripts/bootstrap.sh" /usr/local/lib/maplink-server/bootstrap.sh

if [[ -f /etc/frp-manager/manager.env && ! -f /etc/frp-manager/server.env ]]; then
  sed '/^FRP_MANAGER_ADMIN_HASH=/d' /etc/frp-manager/manager.env > /etc/frp-manager/server.env
  printf '%s\n' 'FRP_MANAGER_ADMIN_HASH_PATH=/etc/frp-manager/admin-password.hash' >> /etc/frp-manager/server.env
  chmod 600 /etc/frp-manager/server.env
fi
if [[ -f /etc/frp-manager/server.env ]] && ! grep -q '^FRP_MANAGER_DEVICES=' /etc/frp-manager/server.env; then
  printf '%s\n' 'FRP_MANAGER_DEVICES=/etc/frp-manager/devices.json' >> /etc/frp-manager/server.env
  chmod 600 /etc/frp-manager/server.env
fi

rm -f /etc/systemd/system/frp-manager.service
systemctl daemon-reload

if [[ "$initialized" == false ]]; then
  /usr/local/lib/maplink-server/bootstrap.sh "$@"
else
  systemctl enable frps.service frp-control.service maplink-server.service
  systemctl restart frps.service
  systemctl restart frp-control.service
  systemctl restart maplink-server.service
  systemctl is-active --quiet frps.service
  systemctl is-active --quiet frp-control.service
  systemctl is-active --quiet maplink-server.service
  cutover_started=false
  trap - ERR
  echo "MapLink Server 已升级并重启，现有配置和证书保持不变。"
  echo "升级备份: $backup_dir"
fi

/usr/local/bin/frp-manager version
/usr/local/bin/frps --version
