# AI Native L3 工程闭环

本仓库把 AI Native 定义为一条工程路径，而不是 AI 代码占比或工具使用次数。

```text
需求/反馈
  → AI 澄清与边界识别
  → 验收标准和测试映射
  → Plan
  → AI/开发者实现
  → 本地 Unit/Integration/E2E
  → AI 自检
  → Pull Request
  → CI Required Checks
  → AI Review + Human Review
  → Merge
  → Build/Release
  → 发布验证
  → 反馈回流
```

## 仓库内证据

| L3 维度 | 工程机制 | 可追溯证据 |
|---|---|---|
| AI First | `AGENTS.md` 与 Issue/PR 模板 | Task/Issue、Plan、PR、Review |
| 验证左移 | Issue 验收标准和测试分层字段 | 编码前的验收与测试映射 |
| 自动化测试 | `scripts/verify.*` 与分层 CI | 本地日志、Actions Jobs |
| AI 闭环 | AGENTS 失败处理规则 | 失败日志、修复提交、重跑结果 |
| 机器门禁 | `main` 分支保护与 Required Checks | Branch Protection、PR Checks |
| AI/Human Review | PR 自检与 CODEOWNERS | Review 记录与处理结果 |
| CI/CD | 标签触发的构建和 Release | Commit、Tag、制品、SHA-256 |
| 反馈闭环 | Bug Issue 模板 | 反馈、修复、测试、发布版本 |

## Required Checks

`main` 合并至少需要：

- Unit tests
- Integration tests
- Build and static checks
- Core E2E
- AI Native evidence

失败时 PR 不允许合并。日常流程不存在第二条进入 `main` 的路径。

## 任务验收记录

每个真实任务至少保留：

- 原始问题与业务目标；
- 明确、可执行的验收标准；
- 修改范围、风险和测试方案；
- AI 参与澄清、实现、自验证或 Review 的记录；
- 本地验证命令与结果；
- CI 失败、修复和重跑（若发生）；
- Human Review 结论；
- Merge Commit、Release 与发布后验证；
- 用户反馈对应的后续 Issue。

## 失败演练

至少定期通过测试分支演练 Unit、Integration 和 Core E2E 失败，确认：

1. PR 自动触发对应检查；
2. 检查失败清楚显示日志；
3. `main` 合并被阻止；
4. 修复推送后检查自动重跑；
5. 全部通过后才恢复可合并状态。

演练不能在 `main` 上制造故障，也不能通过关闭分支保护完成。
