# 贡献指南

MapLink Server 使用 AI Native L3 工作流。贡献的目标不是证明“使用过 AI”，而是让需求、实现、验证和评审形成可追溯闭环。

## 标准流程

1. 从 Feature 或 Bug Issue 开始，写明问题、目标、约束、验收标准和测试分层。
2. 创建功能分支，推荐命名为 `codex/<topic>`、`feature/<topic>` 或 `fix/<topic>`。
3. 在编码前形成 Plan，包含修改范围、实现步骤、风险与验证方法。
4. AI/开发者实现、运行验证、读取失败并继续修复。
5. 至少运行本地单元测试；根据影响范围运行集成、E2E、Build 和静态检查。
6. 执行 AI 自检后创建 PR，完整填写模板并附实际验证结果。
7. CI Required Checks、AI Review 和 Human Review 全部完成后合并。
8. Release 工作流生成可追溯制品；发布后结果和用户反馈继续回流到 Issue。

## 本地验证

Linux/macOS：

```bash
./scripts/verify.sh unit
./scripts/verify.sh all
```

Windows：

```powershell
./scripts/verify.ps1 -Suite unit
./scripts/verify.ps1 -Suite all
```

首次运行 E2E 前安装 Chromium：

```bash
cd web
npx playwright install chromium
```

## 提交和 PR

- 提交应小而完整，消息说明行为变化，例如 `fix: reject replayed remote input`。
- 不提交构建产物、日志、本机配置或任何真实凭据。
- PR 中不要只写“测试通过”；记录执行的命令、结果和未覆盖项。
- 评审意见必须处理或明确回复，不能通过关闭检查绕过。

详细 AI 行为、验证分层和 Definition of Done 见 [AGENTS.md](AGENTS.md)。
