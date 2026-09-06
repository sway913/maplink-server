# MapLink Server

MapLink Server 是映链 MapLink 的自托管服务端。它运行官方原版 `frps`，并提供一个安全的管理控制面，用于配置 FRP、签发客户端连接参数、查看在线状态，以及中转 MapLink 的远程控制会话。

> 本仓库只包含服务端。Windows 与 macOS 客户端位于 [sway913/maplink](https://github.com/sway913/maplink)。

## 功能

- 可视化管理 FRP 控制端口、映射端口段、TLS、连接池与客户端限额。
- 保存配置前先运行 `frps verify`，启动失败时自动恢复旧配置。
- 管理页使用 HTTPS、HttpOnly/SameSite 会话、CSRF 校验和登录限速。
- FRP 原生 Dashboard 只监听 `127.0.0.1`，服务端仅代理允许的只读接口。
- 为 MapLink 客户端提供带 HMAC 校验的在线设备与远程控制中转接口。
- 提供一次性配对码、每设备独立凭据、设备重命名与即时撤销；旧版共享 Token 客户端继续兼容。
- 远程会话、画面、输入与剪贴板文本只保存在内存中，过期后自动清理；支持 720P/30、1080P/60 和 4K/60 画质协商，并通过独立事件通知即时转发鼠标与键盘输入。
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
tar -xzf maplink-server-v0.8.1-linux-amd64.tar.gz
cd maplink-server-v0.8.1-linux-amd64
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

推荐的首次接入流程：

1. 登录管理后台，打开“设备中心”，点击“生成配对码”。
2. 在 MapLink Client 的“连接配置”中填写设备标识、服务器地址和管理端口。
3. 输入 20 位一次性配对码并点击“配对并填充配置”。客户端会自动取得 FRP 连接参数和仅属于本设备的控制凭据。

配对码 10 分钟有效且只能成功使用一次。客户端不会把完整配对码发送到服务器，返回的凭据也使用配对码派生密钥加密，因此可兼容安装程序生成的自签名证书。生产环境仍建议部署可信 HTTPS 证书。

旧版客户端或应急场景仍可手动填写：

- 服务器地址：你的域名或公网 IP；
- 服务器端口：`7000-7010` 中任意已开放端口；
- 管理端口：`7400`；
- Token：登录管理后台后在“连接凭据”中查看；
- 设备 ID：每台设备使用唯一值。

Token 和设备凭据都属于敏感信息，不要提交到 Git、截图或公开日志中。所有需要远控的设备完成配对后，在“设备中心”关闭“旧版远控兼容”，即可强制独立设备身份；此后设备丢失时可单独撤销且不影响其他设备。关闭兼容只影响远控 API，不影响 FRP 端口映射。

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
- [AI Native L3 工程闭环](docs/ai-native-l3.md)
- [AI/Agent 工作规则](AGENTS.md)
- [贡献与 PR 流程](CONTRIBUTING.md)
- [安全策略](SECURITY.md)

## 自动测试与发行

本地统一验证入口：

```bash
./scripts/verify.sh unit
./scripts/verify.sh all
```

Windows 使用 `./scripts/verify.ps1 -Suite unit|all`。提交 PR 前必须至少完成本地 Unit Test。

每次 push 和 Pull Request 都会在 GitHub Actions 分层运行：

- Unit tests
- Integration tests
- Build and static checks
- Core E2E（完整远控协议 + Chromium 管理页）
- AI Native evidence（验收、Plan、验证和 Review 证据）

`main` 只接受 Pull Request，Required Checks 失败时禁止合并。推送 `v*` 标签后，Actions 会在全部检查通过后构建 Linux x86-64/ARM64 发行包、生成 SHA-256 校验文件并创建 GitHub Release。
