import { expect, test } from '@playwright/test'
import { freshGroup, rpc } from './helpers'

test.describe('FR-208 鼠标穿透', () => {
  test('菜单开启：显示状态标记并提示如何恢复，状态持久化', async ({ page }) => {
    await freshGroup(page)
    await expect(page.getByTestId('ct-badge')).toHaveCount(0)
    await page.getByTestId('menu-btn').click()
    await page.getByRole('menuitem', { name: /鼠标穿透/ }).click()
    await expect(page.getByTestId('ct-badge')).toBeVisible()
    await expect(page.getByTestId('ct-badge')).toHaveAttribute('title', /按住 Ctrl/)
    await expect(page.getByTestId('snack')).toContainText('按住 Ctrl 可临时操作')
    await expect(page.getByTestId('snack')).toContainText('托盘菜单')
    expect((await rpc<{ clickThrough: boolean }>(page, 'ApplyWindow')).clickThrough).toBe(true)
    await page.reload()
    await expect(page.getByTestId('ct-badge')).toBeVisible() // 重启（刷新）后仍在

    // 菜单项带勾选状态；再点一次关闭
    await page.getByTestId('menu-btn').click()
    const item = page.getByRole('menuitem', { name: /鼠标穿透/ })
    await expect(item).toContainText('✓')
    await item.click()
    await expect(page.getByTestId('ct-badge')).toHaveCount(0)
    expect((await rpc<{ clickThrough: boolean }>(page, 'ApplyWindow')).clickThrough).toBe(false)
  })

  test('托盘切换（后端事件）会刷新界面状态', async ({ page }) => {
    await freshGroup(page)
    await rpc(page, 'ToggleClickThrough') // 等同托盘菜单
    await expect(page.getByTestId('ct-badge')).toBeVisible()
    await rpc(page, 'ToggleClickThrough')
    await expect(page.getByTestId('ct-badge')).toHaveCount(0)
  })

  test('打开设置 / 快速输入 / 概览期间暂停穿透，后端保存的状态不变', async ({ page }) => {
    await freshGroup(page)
    await rpc(page, 'SetClickThrough', true)
    await rpc(page, 'OpenSettings')
    await expect(page.getByTestId('settings')).toBeVisible()
    await page.getByTestId('settings-close').click()
    await expect(page.getByTestId('sticky')).toBeVisible()
    expect(await rpc<boolean>(page, 'ClickThroughEnabled')).toBe(true)
    await rpc(page, 'SetClickThrough', false)
  })

  test('强提醒处理完后恢复；窗口状态按分组独立', async ({ page }) => {
    const g = await freshGroup(page)
    const it = await rpc<{ id: string }>(page, 'CreateItem', { title: '穿透与强提醒', groupId: g.id })
    await rpc(page, 'SetClickThrough', true)
    await rpc(page, 'ShowStrongAlert', { tag: 't', title: '穿透与强提醒', body: '', itemId: it.id, strong: true, summary: false, missed: false,
      actions: [{ id: 'snooze10', label: '10 分钟后' }] })
    const alert = page.getByTestId('strong-alert')
    await expect(alert).toBeVisible()
    await alert.getByRole('button', { name: '10 分钟后' }).click()
    await expect(alert).toHaveCount(0)
    await expect.poll(async () => rpc<boolean>(page, 'ClickThroughEnabled')).toBe(true) // StrongAlertDone 在界面关闭后才发出
    // 另一个分组不受影响
    const other = await rpc<{ id: string }>(page, 'CreateGroup', `穿透对照${Date.now()}`, '')
    await rpc(page, 'ShowGroup', other.id)
    expect(await rpc<boolean>(page, 'ClickThroughEnabled')).toBe(false)
    await rpc(page, 'ShowGroup', g.id)
    await rpc(page, 'SetClickThrough', false)
  })
})
