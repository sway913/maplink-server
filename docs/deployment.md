# 部署与升级指南

## 安装流程

发行包的 `scripts/install.sh` 只支持 systemd Linux，并要求以 root 运行。参数：

```text
--public-ip ADDRESS   服务器公网 IPv4，可留空后再配置
--admin-user NAME     管理后台用户名，默认 admin
```

安装时会要求输入至少 14 个字符的管理密码。密码只用于本机生成 PBKDF2-SHA256 哈希，不会写入命令历史或环境文件。

主要路径：

| 路径 | 内容 |
|---|---|
| `/usr/local/bin/frps` | 官方 FRP 服务端 |
| `/usr/local/bin/frp-manager` | MapLink Server |
| `/opt/frp-manager/web` | 管理后台静态文件 |
| `/etc/frp/frps.toml` | `frps` 生效配置 |
| `/etc/frp-manager/state.json` | MapLink Server 配置状态，包含 Token |
| `/etc/frp-manager/admin-password.hash` | 管理密码哈希 |
| `/etc/frp-manager/server.env` | 服务环境配置 |
| `/etc/frp-manager/tls.crt` | HTTPS 证书 |
| `/etc/frp-manager/tls.key` | HTTPS 私钥 |

## 防火墙

最小开放范围通常是：

```text
TCP 7400
TCP 7000-7010
UDP 7000
UDP 7002
TCP/UDP 20000-39999（或管理页中配置的映射端口段）
```

`7500` 是原生 FRP Dashboard，仅监听回环地址，不应在安全组或防火墙中开放。

## 使用可信 HTTPS 证书

将证书链和私钥分别部署到：

```text
/etc/frp-manager/tls.crt
/etc/frp-manager/tls.key
```

权限建议：

```bash
sudo chown root:root /etc/frp-manager/tls.crt /etc/frp-manager/tls.key
sudo chmod 644 /etc/frp-manager/tls.crt
sudo chmod 600 /etc/frp-manager/tls.key
sudo systemctl restart maplink-server
```

也可以通过 `FRP_MANAGER_TLS_CERT` 和 `FRP_MANAGER_TLS_KEY` 指定其他路径。客户端必须能够验证域名与证书；生产环境不要长期依赖自签名证书。

## 查看状态和日志

```bash
systemctl status maplink-server frps frp-control
journalctl -u maplink-server -n 200 --no-pager
journalctl -u frps -n 200 --no-pager
```

健康检查：

```bash
curl -k https://127.0.0.1:7400/api/health
```

## 备份

至少备份以下文件：

```text
/etc/frp-manager/state.json
/etc/frp-manager/admin-password.hash
/etc/frp-manager/server.env
/etc/frp-manager/tls.crt
/etc/frp-manager/tls.key
/etc/frp/frps.toml
```

`state.json` 包含客户端 Token，应按敏感凭据保存。不要提交到 GitHub。

## 升级

下载新发行包并再次执行其中的安装脚本。已有状态、密码哈希和证书不会被覆盖，二进制、Web 页面和 systemd unit 会更新，然后服务自动重启。

升级前建议先备份 `/etc/frp-manager` 和 `/etc/frp`。升级后检查：

```bash
/usr/local/bin/frp-manager version
/usr/local/bin/frps --version
curl -k https://127.0.0.1:7400/api/health
```

## 卸载

卸载属于破坏性操作，发行包不会自动提供卸载脚本。确认已备份后，可停止并禁用服务，再手动移除二进制和配置目录。保留 `/etc/frp-manager` 即可保留凭据及配置。
