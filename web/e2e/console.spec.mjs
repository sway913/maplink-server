import { expect, test } from '@playwright/test';

test('未登录用户会看到可操作的安全登录页面', async ({ page }) => {
  await page.goto('/');

  await expect(page).toHaveTitle('映链 MapLink');
  await expect(page.getByRole('heading', { name: '登录服务端' })).toBeVisible();
  await expect(page.getByLabel('用户名')).toHaveValue('admin');
  await expect(page.getByLabel('密码')).toHaveAttribute('type', 'password');
  await expect(page.getByRole('button', { name: '安全登录' })).toBeEnabled();
});

test('登录失败会留在页面并显示明确错误', async ({ page }) => {
  await page.goto('/');
  await page.getByLabel('密码').fill('not-the-real-password');
  await page.getByRole('button', { name: '安全登录' }).click();

  await expect(page.getByRole('button', { name: '安全登录' })).toBeEnabled();
  await expect(page.locator('.alert.error')).toContainText('请求失败');
});

test('静态页面在服务端 CSP 下不依赖内联事件处理器', async ({ page }) => {
  const response = await page.goto('/');
  expect(response?.headers()['content-security-policy']).toContain("script-src-attr 'none'");
  await expect(page.locator('[onclick]')).toHaveCount(0);
});
