import { pathToFileURL } from 'node:url';

const requiredSections = [
  '需求与目标',
  '验收标准',
  '实现计划',
  '修改说明',
  '验证证据',
  'AI 自检',
  '人工评审',
];

export function validatePullRequestBody(body) {
  const text = String(body || '').replace(/\r\n/g, '\n');
  const errors = [];

  for (const section of requiredSections) {
    const escaped = section.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
    if (!new RegExp(`^#{2,3}\\s+${escaped}\\s*$`, 'mi').test(text)) {
      errors.push(`缺少章节：${section}`);
    }
  }

  if (!/-\s*\[[ xX]\]\s+.+/.test(text)) {
    errors.push('验收标准必须至少包含一个 Markdown 复选项');
  }
  if (/<!--\s*(请填写|必填)/.test(text)) {
    errors.push('仍有未填写的必填占位内容');
  }
  if (!/(verify\.(sh|ps1)|go test|npm (test|run)|playwright)/i.test(text)) {
    errors.push('验证证据必须包含实际执行的测试命令');
  }

  return errors;
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  const errors = validatePullRequestBody(process.env.PR_BODY || '');
  if (errors.length) {
    console.error('AI Native PR 证据检查失败：');
    for (const error of errors) console.error(`- ${error}`);
    process.exit(1);
  }
  console.log('AI Native PR 证据完整。');
}
