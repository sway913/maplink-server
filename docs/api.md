# HTTP API

所有接口默认通过 `https://SERVER:7400` 提供。除健康检查外，不建议让非 MapLink 客户端直接调用。

## 健康检查

`GET /api/health` 无需认证，返回服务状态、版本和功能列表。

## 管理后台接口

管理接口先调用 `POST /api/auth/login` 获取 HttpOnly 会话 Cookie。所有修改操作还必须携带登录响应中的 `X-CSRF-Token`。

| 方法与路径 | 用途 |
|---|---|
| `POST /api/auth/login` | 登录 |
| `GET /api/auth/session` | 获取会话与 CSRF Token |
| `POST /api/auth/logout` | 注销 |
| `POST /api/auth/password` | 修改管理密码并注销旧会话 |
| `GET /api/system` | 系统与 frps 状态 |
| `GET /api/config` | 读取脱敏配置 |
| `PUT /api/config` | 校验并应用配置 |
| `GET /api/credentials` | 获取客户端连接参数 |
| `POST /api/credentials/rotate` | 轮换客户端 Token |
| `GET /api/devices` | 查看已配对及在线兼容设备，不返回设备凭据 |
| `POST /api/devices/enrollments` | 生成 10 分钟有效的一次性配对码 |
| `PUT /api/devices/policy` | 开关共享 Token 的旧版远控兼容 |
| `PATCH /api/devices/{deviceID}` | 重命名已配对设备 |
| `DELETE /api/devices/{deviceID}` | 撤销设备凭据并关闭相关远控会话 |
| `POST /api/service` | start/stop/restart frps |
| `GET /api/logs` | 读取受限的 frps journal |
| `GET /api/ports` | 查看监听端口 |
| `GET /api/frp/{resource}` | 代理允许的 FRP 只读监控接口 |

## 客户端在线设备接口

`GET /api/client/devices` 使用 FRP Token 进行 HMAC-SHA256 验证：

```text
payload = "GET\n/api/client/devices\n" + unix_timestamp
signature = hex(HMAC-SHA256(token, payload))
```

请求头：

```text
X-MapLink-Timestamp: Unix 秒
X-MapLink-Signature: 十六进制签名
```

时间偏差超过 120 秒的请求会被拒绝。

## 客户端设备配对

`POST /api/client/enroll` 不需要管理会话，但必须证明持有管理后台生成的一次性配对码。完整配对码规范化为 20 位大写字符后仅在客户端使用：

```text
key = SHA-256(normalized_pairing_code)
code_id = normalized_pairing_code[0:5]
proof = hex(HMAC-SHA256(key, device_id + "\n" + name + "\n" + platform + "\n" + nonce))
```

请求只发送 `codeID`、随机 `nonce`、`proof`、`deviceID`、`name` 和 `platform`，不发送完整配对码。成功响应使用 AES-256-GCM 加密，密钥为上述 `key`，附加认证数据固定为 `maplink-device-enrollment-v1`；响应包含 Base64URL 编码的 `nonce` 和 `ciphertext`。解密后得到 FRP 参数及每设备独立控制凭据。配对码在成功注册后立即失效，错误证明不会消耗配对码。

## 远程控制中转接口

远程控制请求使用带时间戳、随机数和正文摘要的 HMAC：

```text
body_hash = hex(SHA-256(raw_body))
payload = method + "\n" + request_uri + "\n" + timestamp + "\n" + nonce + "\n" + body_hash
signature = hex(HMAC-SHA256(token, payload))
```

请求头：

```text
X-MapLink-Timestamp: Unix 秒
X-MapLink-Nonce: 16-96 字符的一次性随机值
X-MapLink-Signature: 十六进制签名
X-MapLink-Device-ID: 已配对设备 ID（新版客户端）
```

新版客户端使用该设备的独立凭据计算签名，服务端同时校验设备身份及会话角色；旧客户端不发送设备 ID 时默认继续使用共享 FRP Token。所有设备完成配对后，管理员应通过设备中心关闭旧版远控兼容，才能强制执行设备撤销和身份隔离。该策略持久化在设备注册表中，关闭后不影响 FRP 端口映射。服务端会拒绝已撤销设备、身份冒用、角色越权、超时、签名错误或重复 Nonce 的请求。主要接口包括在线主机心跳、会话创建/接受/关闭、画面上传/下载以及输入事件队列。

远程画面和输入只在内存中短暂保留。接口不用于长期录像、文件储存或私钥传输。
