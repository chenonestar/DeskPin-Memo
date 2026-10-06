import { expect, test } from '@playwright/test'
import { freshGroup, rpc } from './helpers'

test.describe('注册表痕迹（绿色版）', () => {
  test('列出本程序写入 HKCU 的项，清除前需确认，清除后关闭开机自启', async ({ page }) => {
    await freshGroup(page)
    // 开发服务器用内存注册表模拟：预置了自启、通知身份、协议三项
    const s = await rpc<Record<string, unknown>>(page, 'GetSettings')
    await rpc(page, 'SaveSettings', { ...s, autostart: true })
    await rpc(page, 'OpenSettings')
    await page.getByRole('tab', { name: '数据' }).click()

    const items = page.getByTestId('reg-item')
    await expect(items).toHaveCount(3)
    await expect(page.getByTestId('reg-list')).toContainText('开机自启')
    await expect(page.getByTestId('reg-list')).toContainText('通知身份')
    await expect(page.getByTestId('reg-list')).toContainText('deskpin:// 协议')
    await expect(page.getByTestId('reg-list')).toContainText('HKCU\\Software\\Microsoft\\Windows\\CurrentVersion\\Run')
    await expect(page.getByText('只会写入当前用户注册表')).toBeVisible()

    // 取消不会清除
    await page.getByTestId('reg-clear').click()
    await expect(page.getByRole('dialog')).toContainText('将删除上面列出的 3 项')
    await page.getByRole('dialog').getByRole('button', { name: '取消' }).click()
    await expect(items).toHaveCount(3)

    // 确认清除
    await page.getByTestId('reg-clear').click()
    await page.getByRole('dialog').getByRole('button', { name: '清除' }).click()
    await expect(page.getByTestId('reg-done')).toContainText('已删除 3 项')
    await expect(page.getByTestId('reg-quit')).toBeVisible()
    await page.getByRole('button', { name: '继续使用' }).click()
    await expect(page.getByTestId('reg-empty')).toBeVisible()
    await expect(page.getByTestId('reg-clear')).toBeDisabled()
    expect((await rpc<{ autostart: boolean }>(page, 'GetSettings')).autostart).toBe(false)
    expect(await rpc<unknown[]>(page, 'RegistryTraces')).toEqual([])
  })
})
