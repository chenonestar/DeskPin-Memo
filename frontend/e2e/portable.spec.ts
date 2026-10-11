import { expect, test, type Page } from '@playwright/test'
import { freshGroup, rpc } from './helpers'

// 模拟绿色版的开发服务器（8791，内存注册表）：默认零注册表写入，「系统集成」是显式开关
test.use({ baseURL: 'http://127.0.0.1:8791' })

const traces = (page: Page) => rpc<{ key: string }[]>(page, 'RegistryTraces')
const setSettings = async (page: Page, patch: Record<string, unknown>) => {
  const s = await rpc<Record<string, unknown>>(page, 'GetSettings')
  await rpc(page, 'SaveSettings', { ...s, ...patch })
}

test.describe('绿色版：默认零注册表写入，系统集成需手动启用', () => {
  test.afterEach(async ({ page }) => { await setSettings(page, { systemIntegration: false, autostart: false }) })

  test('默认状态：没有任何注册表项，自启不可用并说明原因', async ({ page }) => {
    await freshGroup(page)
    expect(await traces(page)).toEqual([])
    // 即使强行保存 autostart=true 也会被压回 false，且不写注册表
    await setSettings(page, { autostart: true, fadeOnLeave: false })
    expect((await rpc<{ autostart: boolean; systemIntegration: boolean }>(page, 'GetSettings'))).toMatchObject({ autostart: false, systemIntegration: false })
    expect(await traces(page)).toEqual([])

    await rpc(page, 'OpenSettings')
    await expect(page.getByLabel('启用系统集成', { exact: false })).not.toBeChecked()
    await expect(page.getByLabel('开机自动启动（静默显示便签）')).toBeDisabled()
    await expect(page.getByTestId('integration-hint')).toContainText('不向注册表写入任何内容')
    await expect(page.getByTestId('portable-autostart-hint')).toContainText('请先启用')
    await page.getByRole('tab', { name: '数据' }).click()
    await expect(page.getByTestId('reg-empty')).toBeVisible()
    await expect(page.getByTestId('reg-clear')).toBeDisabled()
  })

  test('启用系统集成才写入通知身份和协议；再开自启才写 Run；关闭集成立即全部清除', async ({ page }) => {
    await freshGroup(page)
    await rpc(page, 'OpenSettings')
    await page.getByLabel('启用系统集成', { exact: false }).check()
    await expect.poll(async () => (await traces(page)).map((t) => t.key).join('|')).toMatch(/AppUserModelId.*deskpin|deskpin.*AppUserModelId/)
    expect((await traces(page)).some((t) => t.key.endsWith('\\Run\\DeskPinMemo'))).toBe(false) // 没开自启就不写 Run
    await expect(page.getByLabel('开机自动启动（静默显示便签）')).toBeEnabled()

    await page.getByLabel('开机自动启动（静默显示便签）').check()
    await expect.poll(async () => (await traces(page)).some((t) => t.key.endsWith('\\Run\\DeskPinMemo'))).toBe(true)

    await page.getByLabel('启用系统集成', { exact: false }).uncheck()
    await expect.poll(async () => (await traces(page)).length).toBe(0)
    expect(await rpc<{ autostart: boolean }>(page, 'GetSettings')).toMatchObject({ autostart: false })
    await expect(page.getByLabel('开机自动启动（静默显示便签）')).toBeDisabled()
  })

  test('「清除注册表项」在绿色版同时关闭系统集成，之后不会被重新写入', async ({ page }) => {
    await freshGroup(page)
    await setSettings(page, { systemIntegration: true })
    await setSettings(page, { autostart: true })
    expect((await traces(page)).length).toBe(3)
    await rpc(page, 'OpenSettings')
    await page.getByRole('tab', { name: '数据' }).click()
    await page.getByTestId('reg-clear').click()
    await page.getByRole('dialog').getByRole('button', { name: '清除' }).click()
    await expect(page.getByTestId('reg-done')).toContainText('已删除 3 项')
    expect(await rpc<{ systemIntegration: boolean; autostart: boolean }>(page, 'GetSettings')).toMatchObject({ systemIntegration: false, autostart: false })
    // 之后改别的设置也不会把它们写回去
    await setSettings(page, { theme: 'dark' })
    expect(await traces(page)).toEqual([])
    await setSettings(page, { theme: 'system' })
  })
})
