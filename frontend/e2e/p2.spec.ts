import { expect, test } from '@playwright/test'
import { mkdtempSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { freshGroup, rpc } from './helpers'

test.describe('FR-308 每日概览', () => {
  test('展示逾期与今天到期的事项，可勾选完成', async ({ page }) => {
    const g = await freshGroup(page)
    const now = Date.now()
    const tag = Date.now()
    const overdue = await rpc<{ id: string }>(page, 'CreateItem', { title: `概览逾期${tag}`, groupId: g.id, dueAt: now - 26 * 3600000 })
    await rpc(page, 'CreateItem', { title: `概览今天${tag}`, groupId: g.id, dueAt: now + 60_000 })
    await rpc(page, 'CreateItem', { title: `概览明天${tag}`, groupId: g.id, dueAt: now + 40 * 3600000 })
    await rpc(page, 'CreateItem', { title: `概览无日期${tag}`, groupId: g.id })

    await rpc(page, 'OpenOverview')
    const ov = page.getByTestId('overview')
    await expect(ov).toBeVisible()
    await expect(ov).toContainText('今日概览')
    const over = page.getByTestId('overview-overdue').getByTestId('overview-row').filter({ hasText: `概览逾期${tag}` })
    await expect(over).toHaveCount(1)
    await expect(over.locator('.due')).toHaveCSS('color', 'rgb(217, 48, 37)')
    await expect(page.getByTestId('overview-today').getByTestId('overview-row').filter({ hasText: `概览今天${tag}` })).toHaveCount(1)
    // 明天和无日期的不在概览里
    await expect(ov.getByText(`概览明天${tag}`)).toHaveCount(0)
    await expect(ov.getByText(`概览无日期${tag}`)).toHaveCount(0)

    // 在概览里直接勾选完成
    await over.getByRole('checkbox').click()
    await expect(over).toHaveClass(/done/)
    await expect.poll(async () => {
      const gv = await rpc<{ done: { id: string }[] }>(page, 'GroupContent', g.id)
      return gv.done.some((i) => i.id === overdue.id)
    }).toBe(true)

    await page.getByTestId('overview-close').click()
    await expect(ov).toHaveCount(0)
    await expect(page.getByTestId('sticky')).toBeVisible()
  })

  test('Esc 关闭；「不再每天弹出」会关闭设置', async ({ page }) => {
    await freshGroup(page)
    await rpc(page, 'OpenOverview')
    await expect(page.getByTestId('overview')).toBeVisible()
    await page.keyboard.press('Escape')
    await expect(page.getByTestId('overview')).toHaveCount(0)

    const before = await rpc<{ dailyOverview: boolean }>(page, 'GetSettings')
    expect(before.dailyOverview).toBe(true) // 默认开启
    await rpc(page, 'OpenOverview')
    await page.getByTestId('overview-off').click()
    await expect(page.getByTestId('overview')).toHaveCount(0)
    expect((await rpc<{ dailyOverview: boolean }>(page, 'GetSettings')).dailyOverview).toBe(false)
    // 恢复，避免影响其他用例
    await rpc(page, 'SaveSettings', { ...(await rpc<Record<string, unknown>>(page, 'GetSettings')), dailyOverview: true })
  })

  test('设置页「提醒」里有开关并持久化', async ({ page }) => {
    await freshGroup(page)
    await rpc(page, 'OpenSettings')
    await page.getByRole('tab', { name: '提醒' }).click()
    const box = page.getByLabel('每天首次启动时弹出今日事项概览')
    await expect(box).toBeChecked()
    await box.uncheck()
    await expect.poll(async () => (await rpc<{ dailyOverview: boolean }>(page, 'GetSettings')).dailyOverview).toBe(false)
    await box.check()
    await expect.poll(async () => (await rpc<{ dailyOverview: boolean }>(page, 'GetSettings')).dailyOverview).toBe(true)
  })

  test('后端：MaybeShowOverview 当天只认领一次', async ({ page }) => {
    await freshGroup(page)
    const first = await rpc<boolean>(page, 'MaybeShowOverview')
    const second = await rpc<boolean>(page, 'MaybeShowOverview')
    expect(second).toBe(false)
    if (first) await expect(page.getByTestId('overview')).toBeVisible({ timeout: 5000 })
    await rpc(page, 'CloseOverview')
  })
})

test.describe('FR-605 数据目录', () => {
  const open = async (page: import('@playwright/test').Page) => {
    await freshGroup(page)
    await rpc(page, 'OpenSettings')
    await page.getByRole('tab', { name: '数据' }).click()
    await page.getByTestId('datadir-change').click()
  }

  test('校验：相对路径、当前目录给出错误', async ({ page }) => {
    await open(page)
    const input = page.getByTestId('datadir-input')
    await input.fill('relative/path')
    await expect(page.getByTestId('datadir-error')).toContainText('完整路径')
    await expect(page.getByTestId('datadir-apply')).toBeDisabled()
    await input.fill(await page.getByTestId('datadir-input').inputValue()) // no-op
    const cur = await rpc<{ dir: string }>(page, 'DataDirStatus')
    await input.fill(cur.dir)
    await expect(page.getByTestId('datadir-error')).toContainText('已经是当前')
  })

  test('复制到新目录：确认后提示重启，原数据保留', async ({ page }) => {
    const target = join(mkdtempSync(join(tmpdir(), 'deskpin-e2e-')), 'Sync', 'DeskPinMemo')
    await open(page)
    await page.getByTestId('datadir-input').fill(target)
    await expect(page.getByTestId('datadir-copy-note')).toContainText('复制')
    await page.getByTestId('datadir-apply').click()
    const done = page.getByTestId('datadir-done')
    await expect(done).toContainText(target)
    await expect(done).toContainText('重启后生效')
    await expect(page.getByTestId('datadir-restart')).toBeVisible()
    // 重启前仍在使用原目录
    const st = await rpc<{ dir: string; custom: boolean }>(page, 'DataDirStatus')
    expect(st.dir).not.toBe(target)
  })

  test('目标已有数据：必须选择「使用已有」或「替换」，不会静默覆盖', async ({ page }) => {
    const target = mkdtempSync(join(tmpdir(), 'deskpin-e2e-'))
    // 先把当前数据复制过去，让目标目录里出现 data.db
    await freshGroup(page)
    await rpc(page, 'ChangeDataDir', target, 'copy')
    await open(page)
    await page.getByTestId('datadir-input').fill(target)
    const conflict = page.getByTestId('datadir-conflict')
    await expect(conflict).toContainText('已经有一份 data.db')
    await expect(page.getByLabel(/使用目录里已有的数据/)).toBeChecked()
    await page.getByLabel(/用当前数据替换/).check()
    await page.getByTestId('datadir-apply').click()
    await expect(page.getByTestId('datadir-done')).toContainText('改名保留')
  })

  test('网盘目录给出提示', async ({ page }) => {
    const target = join(mkdtempSync(join(tmpdir(), 'deskpin-e2e-')), 'OneDrive', 'DeskPinMemo')
    await open(page)
    await page.getByTestId('datadir-input').fill(target)
    await expect(page.getByTestId('datadir-warning')).toContainText('网盘同步目录')
    await expect(page.getByTestId('datadir-apply')).toBeEnabled()
  })
})
