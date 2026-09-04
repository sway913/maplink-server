import assert from 'node:assert/strict';
import test from 'node:test';
import { validatePullRequestBody } from '../scripts/check-pr-evidence.mjs';

const completeBody = `
## 需求与目标
让任务可追溯。
## 验收标准
- [x] Required Checks 全部通过
## 实现计划
先测试再修改。
## 修改说明
增加工程门禁。
## 验证证据
执行 ./scripts/verify.sh unit，结果通过。
## AI 自检
已检查安全和回归。
## 人工评审
等待 CODEOWNER 评审。
`;

test('完整 PR 证据可以通过检查', () => {
  assert.deepEqual(validatePullRequestBody(completeBody), []);
});

test('缺少验收与验证命令会失败', () => {
  const errors = validatePullRequestBody('## 需求与目标\n只有目标');
  assert.ok(errors.some((error) => error.includes('验收标准')));
  assert.ok(errors.some((error) => error.includes('测试命令')));
});
