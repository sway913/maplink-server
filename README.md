# MapLink Server

MapLink Server 是映链 MapLink 的自托管服务端。它运行官方原版 `frps`，并提供一个安全的管理控制面，用于配置 FRP、签发客户端连接参数、查看在线状态，以及中转 MapLink 的远程控制会话。

> 本仓库只包含服务端。Windows 与 macOS 客户端位于 [sway913/maplink](https://github.com/sway913/maplink)。

## 功能

- 可视化管理 FRP 控制端口、映射端口段、TLS、连接池与客户端限额。
- 保存配置前先运行 `frps verify`，启动失败时自动恢复旧配置。
- 管理页使用 HTTPS、HttpOnly/SameSite 会话、CSRF 校验和登录限速。
- FRP 原生 Dashboard 只监听 `127.0.0.1`，服务端仅代理允许的只读接口。
- 为 MapLink 客户端提供带 HMAC 校验的在线设备与远程控制中转接口。
- 远程会话、画面和输入队列只保存在内存中，过期后自动清理。
- 只中转 MapLink 专用 SSH 公钥；不接收、不保存 SSH 私钥和登录密码。
- 支持 Linux x86-64 与 ARM64，发行包内置匹配版本的官方 `frps`。

## 架构

```text
MapLink Client ── FRP 控制/工作连接 ── frps
      │                                  │
      └── HTTPS + HMAC ── MapLink Server ┘
                              │
                              ├── 管理后台静态页面
                              ├── 配置校验与失败回滚
                              ├── systemd / nftables 管理
                              └── 内存态远程控制中转
```

MapLink Server 不替换或修改 FRP 协议。端口映射仍由官方 `frps`/`frpc` 完成。

## 系统要求

- 64 位 Linux（x86-64 或 ARM64）
- systemd
- nftables
- OpenSSL
- `ss`（通常由 `iproute2` 提供）
- 对外可访问的公网地址

推荐使用 Debian 12、Ubuntu 24.04 LTS 或兼容发行版。

## 快速安装

1. 从 [Releases](https://github.com/sway913/maplink-server/releases) 下载与服务器架构匹配的压缩包。
2. 解压并以 root 运行安装脚本：

```bash
tar -xzf maplink-server-v0.6.1-linux-amd64.tar.gz
cd maplink-server-v0.6.1-linux-amd64
sudo ./scripts/install.sh --public-ip 203.0.113.10
```

安装程序会：

- 安装 `frps` 与 `frp-manager`；
- 安装并启用 systemd 服务；
- 生成随机 FRP Token 和内部 Dashboard 密码；
- 交互式创建管理后台密码；
- 生成初始配置和自签名 HTTPS 证书；
- 配置 nftables 多控制端口入口。

完成后访问：

```text
https://服务器地址:7400
```

自签名证书会触发浏览器警告。正式环境应替换为可信证书，详见[部署与升级指南](docs/deployment.md)。

## 默认端口

| 端口 | 协议 | 用途 |
|---|---|---|
| `7400` | TCP/HTTPS | MapLink 管理页及客户端 API |
| `7000-7010` | TCP | FRP 客户端控制入口，额外端口由 nftables 转发到主入口 |
| `7000` | UDP | KCP |
| `7002` | UDP | QUIC |
| `7500` | TCP/本机 | FRP 原生 Dashboard，不应暴露公网 |
| `20000-39999` | TCP/UDP | 默认远程映射端口段 |

安装前请根据实际需求调整防火墙。不要将 `7500` 开放到公网。

## 客户端接入

在 MapLink Client 的连接设置中填写：

- 服务器地址：你的域名或公网 IP；
- 服务器端口：`7000-7010` 中任意已开放端口；
- 管理端口：`7400`；
- Token：登录管理后台后在“连接凭据”中查看；
- 设备 ID：每台设备使用唯一值。

Token 相当于客户端接入密码，不要提交到 Git、截图或公开日志中。

## 从源码构建

需要 Go 1.25 与 Node.js 22 或更高版本：

```bash
go test ./...
go vet ./...
CGO_ENABLED=0 go build -trimpath -o bin/frp-manager ./cmd/frp-manager

cd web
npm ci
npm run lint
npm test
npm run build:static
```

管理后台静态文件输出到 `web/out`。运行服务时通过 `FRP_MANAGER_WEB_ROOT` 指向该目录。

## 文档

- [部署、证书、备份与升级](docs/deployment.md)
- [HTTP API 与认证方式](docs/api.md)
- [安全策略](SECURITY.md)

## 自动测试与发行

每次 push 和 Pull Request 都会在 GitHub Actions 运行：

- `go test -race ./...`
- `go vet ./...`
- Web ESLint、单元测试与静态构建
- 敏感信息扫描

推送 `v*` 标签后，Actions 会在全部测试通过后构建 Linux x86-64/ARM64 发行包、生成 SHA-256 校验文件并创建 GitHub Release。
