# MapLink Server Agent Guide

本文件适用于整个仓库。目标是让人和 AI 按同一套可验证、可追溯、不可随意绕过的工程流程协作。

## 研发原则

1. **AI First**：新任务默认先让 AI 参与需求澄清、边界识别、方案、计划、测试、实现、自检和反馈分析，不把 AI 限定为代码生成器。
2. **Validation First**：编码前必须明确验收标准，以及每条标准对应的 Unit、Integration、E2E、Build 或静态验证。
3. **Closed Loop**：生成代码不等于完成。AI 必须读取验证结果，修复失败并重新运行，直到必要验证全部通过。
4. **Machine Gate**：正常修改只通过 Pull Request 进入 `main`；Required Checks 失败时不得合并。
5. **Evidence & Human Control**：PR 必须能追溯需求、计划、修改、测试和 AI 自检；关键代码仍由人最终评审。

## 开始任务前

- 阅读 `README.md`、相关代码与测试，不猜测现有行为。
- 用以下字段澄清任务；能从上下文确定时直接写入 Plan，不重复询问：
  - 原始问题与业务目标；
  - 使用场景和不在范围内的事项；
  - 安全、兼容性、性能和部署约束；
  - 可执行的验收标准；
  - Unit / Integration / Core E2E 验证映射。
- 修改超过一个简单局部时，先形成短 Plan，至少包含范围、步骤、测试、风险和回滚方式。

## 实现与验证闭环

1. 优先增加或更新能够复现需求/缺陷的测试。
2. 只修改满足验收标准所需的最小范围，保留无关用户改动。
3. 每个有意义的修改批次后运行最小相关测试。
4. 提交前必须运行本地单元测试；完整任务还应运行集成和核心 E2E。
5. 测试失败时读取真实日志并定位根因，不通过删除断言、跳过测试或弱化门禁来“修复”。
6. 验证全部通过后执行 AI 自检：需求覆盖、边界、安全、回归、测试充分性、无关修改和文档同步。

统一验证入口：

```bash
./scripts/verify.sh unit
./scripts/verify.sh integration
./scripts/verify.sh e2e
./scripts/verify.sh all
```

Windows PowerShell：

```powershell
./scripts/verify.ps1 -Suite unit
./scripts/verify.ps1 -Suite integration
./scripts/verify.ps1 -Suite e2e
./scripts/verify.ps1 -Suite all
```

如果缺少 Go、Node.js 或 Playwright 浏览器，明确报告缺失依赖；不要宣称对应测试已通过。云端 CI 不能替代提交前的本地单元测试。

## 测试分层

- **Unit**：`internal/auth`、`internal/frp`、`internal/version` 和 Web 纯逻辑测试；快速、隔离、无网络。
- **Integration**：`internal/manager` 中管理 API、配置写入、失败回滚、systemd/nft 渲染和认证边界测试。
- **Core E2E**：完整远控中转协议测试，以及 Chromium 中真实加载静态管理页、登录失败和 CSP 行为。
- **Build / Static**：`go vet`、Linux 编译、ESLint、Next.js 静态构建、脚本语法和敏感信息扫描。

修改核心行为时必须选择正确层级，不能只添加字符串匹配测试代替可执行行为验证。

## 安全边界

- 永远不要提交或输出真实服务器 IP、域名、Token、密码、Cookie、TLS 私钥或 SSH 私钥。
- 文档和测试使用 `203.0.113.0/24`、`example.com` 与明显的测试凭据。
- SSH 私钥永远不离开创建它的设备；远控协议只允许中转格式有效的 Ed25519 公钥。
- 对认证、HMAC、Nonce、防重放、会话、CSRF、配置回滚、端口校验、TLS、文件权限或命令执行的修改视为高风险，必须增加负向测试。
- 不增加任意 Shell/命令执行 API，不把 FRP Dashboard 暴露到公网。
- API 正文、帧、队列和日志读取必须保持大小/数量上限。

## Pull Request 规则

- 从 `codex/<topic>` 或其他功能分支创建 PR，不直接推送 `main`。
- 使用仓库 PR 模板，填写需求目标、验收标准、Plan、修改说明、验证证据、AI 自检和人工评审。
- 验证证据必须包含实际命令与结果；未运行项目要写明原因，不能留空。
- AI Review 与 CI 职责不同：AI Review 找逻辑、测试和安全风险；CI 证明可执行条件实际通过。两者都需要。
- 不因“改动很小”跳过测试。紧急处理必须留下原因、责任人、时间、后补测试和回滚记录。

## Definition of Done

仅当以下条件全部满足时才能声明完成：

- 验收标准逐条对应到实现和可执行验证；
- 本地 Unit Test 已通过；
- 必要的 Integration、Core E2E、Build 和 Static Check 已通过；
- AI 自检未发现未处理的高风险问题；
- 文档、示例、部署或 API 契约已同步；
- 无真实凭据或私钥进入 Git；
- PR Required Checks 全绿并完成要求的人工 Review；
- 发布任务包含制品校验和发布后验证证据。

## 版本与发布

- 版本采用语义化版本，Release 标签为 `v<version>`。
- 只有受保护的 `main` 上已验证提交可以打标签。
- GitHub Actions 必须在 Unit、Integration、Build/Static、Core E2E 和 AI Native Evidence 全部通过后才能生成发行包。
- 发布后验证 Release 资产数量、SHA-256、二进制版本和健康检查契约；反馈问题重新进入同一任务与 PR 闭环。
