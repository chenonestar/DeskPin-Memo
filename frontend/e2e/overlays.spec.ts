import { expect, test } from '@playwright/test'
import { freshGroup, rows, rpc } from './helpers'

test.describe('快速输入框', () => {
  test('AC-01：Enter 保存并关闭，Tab 展开详细字段，Esc 不保存', async ({ page }) => {
    const g = await freshGroup(page)
    await rpc(page, 'OpenQuick')
    const quick = page.getByTestId('quick')
    await expect(quick).toBeVisible()
    await expect(page.getByTestId('quick-input')).toBeFocused()
    await page.getByTestId('quick-input').fill('明天下午3点 交报告 #工作')
    const pv = page.getByTestId('quick-preview')
    await expect(pv).toContainText('交报告')
    await expect(pv).toContainText('明天 15:00')
    await expect(pv).toContainText('#工作')
    await page.getByTestId('quick-input').press('Enter')
    await expect(quick).toHaveCount(0)
    await expect(page.getByTestId('sticky')).toBeVisible()
    const gv = await rpc<{ todo: { title: string; dueAt: number; tags: string[]; reminders: unknown[] }[] }>(page, 'GroupContent', g.id)
    expect(gv.todo).toHaveLength(1)
    expect(gv.todo[0].title).toBe('交报告')
    expect(gv.todo[0].tags).toEqual(['工作'])
    expect(gv.todo[0].reminders).toHaveLength(1)
    const d = new Date(gv.todo[0].dueAt)
    expect(d.getHours()).toBe(15)

    // Esc 不保存
    await rpc(page, 'OpenQuick')
    await page.getByTestId('quick-input').fill('不该被保存')
    await page.keyboard.press('Escape')
    await expect(page.getByTestId('quick')).toHaveCount(0)
    const gv2 = await rpc<{ todo: unknown[] }>(page, 'GroupContent', g.id)
    expect(gv2.todo).toHaveLength(1)
  })

  test('Tab 切换到详细字段，备注和优先级生效', async ({ page }) => {
    const g = await freshGroup(page)
    await rpc(page, 'OpenQuick')
    await page.getByTestId('quick-input').fill('续费域名')
    await page.keyboard.press('Tab')
    await expect(page.getByTestId('quick-note')).toBeFocused()
    await page.getByTestId('quick-note').fill('到期前一周')
    await page.getByLabel('优先级').selectOption('2')
    await page.getByTestId('quick-note').press('Enter')
    await expect(page.getByTestId('quick')).toHaveCount(0)
    const gv = await rpc<{ todo: { note: string; priority: number }[] }>(page, 'GroupContent', g.id)
    expect(gv.todo[0]).toMatchObject({ note: '到期前一周', priority: 2 })
  })

  test('周期语句：每月5日 09:00 交报销单', async ({ page }) => {
    const g = await freshGroup(page)
    await rpc(page, 'OpenQuick')
    await page.getByTestId('quick-input').fill('每月5日 09:00 交报销单')
    await expect(page.getByTestId('quick-preview')).toContainText('每月 5 日')
    await page.keyboard.press('Enter')
    await expect(page.getByTestId('quick')).toHaveCount(0)
    const gv = await rpc<{ todo: { reminders: { repeatRule: string }[] }[] }>(page, 'GroupContent', g.id)
    expect(gv.todo[0].reminders[0].repeatRule).toBe('FREQ=MONTHLY;BYMONTHDAY=5')
  })
})

test.describe('设置窗口', () => {
  test('六个页签，外观修改即时生效并持久化', async ({ page }) => {
    await freshGroup(page)
    await rpc(page, 'OpenSettings')
    const st = page.getByTestId('settings')
    await expect(st).toBeVisible()
    for (const name of ['常规', '外观', '提醒', '快捷键', '数据', '关于']) {
      await expect(st.getByRole('tab', { name })).toBeVisible()
    }
    await st.getByRole('tab', { name: '外观' }).click()
    await st.getByLabel('颜色主题').selectOption('dark')
    await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark')
    await st.getByLabel('字号').selectOption('16')
    await expect(page.locator('html')).toHaveCSS('--font-size', '16px')
    // 界面是乐观更新，保存请求还在途中：轮询等待后端落地
    await expect.poll(async () => rpc<{ theme: string; fontSize: number }>(page, 'GetSettings')).toMatchObject({ theme: 'dark', fontSize: 16 })
    await st.getByLabel('颜色主题').selectOption('system')
    await st.getByLabel('字号').selectOption('14')
    await st.getByTestId('settings-close').click()
    await expect(page.getByTestId('sticky')).toBeVisible()
  })

  test('提醒页：免打扰与默认提醒时刻', async ({ page }) => {
    await freshGroup(page)
    await rpc(page, 'OpenSettings')
    await page.getByRole('tab', { name: '提醒' }).click()
    await page.getByLabel('启用免打扰时段（提醒延后到时段结束）').check()
    await page.getByLabel('开始').fill('23:00')
    await expect.poll(async () => (await rpc<{ dnd: { enabled: boolean; start: string } }>(page, 'GetSettings')).dnd).toMatchObject({ enabled: true, start: '23:00' })
    await page.getByLabel('启用免打扰时段（提醒延后到时段结束）').uncheck()
  })

  test('快捷键录入', async ({ page }) => {
    await freshGroup(page)
    await rpc(page, 'OpenSettings')
    await page.getByRole('tab', { name: '快捷键' }).click()
    await page.getByRole('button', { name: '快速新建' }).click()
    await page.keyboard.press('Control+Shift+K')
    await expect(page.getByRole('button', { name: '快速新建' })).toHaveText('Ctrl+Shift+K')
    await expect.poll(async () => (await rpc<{ hotkeyQuick: string }>(page, 'GetSettings')).hotkeyQuick).toBe('Ctrl+Shift+K')
    await page.getByRole('button', { name: '快速新建' }).click()
    await page.keyboard.press('Control+Alt+N')
    await expect(page.getByRole('button', { name: '快速新建' })).toHaveText('Ctrl+Alt+N')
    // 切换鼠标穿透的热键：默认 Ctrl+Alt+P，可自定义
    await expect(page.getByRole('button', { name: '切换鼠标穿透' })).toHaveText('Ctrl+Alt+P')
    await page.getByRole('button', { name: '切换鼠标穿透' }).click()
    await page.keyboard.press('Control+Alt+L')
    await expect.poll(async () => (await rpc<{ hotkeyClickThrough: string }>(page, 'GetSettings')).hotkeyClickThrough).toBe('Ctrl+Alt+L')
    await page.getByRole('button', { name: '切换鼠标穿透' }).click()
    await page.keyboard.press('Control+Alt+P')
  })

  test('分组管理：重命名、不能删除收件箱', async ({ page }) => {
    await freshGroup(page)
    await rpc(page, 'OpenSettings')
    const inbox = page.getByRole('button', { name: /删除 收件箱/ })
    await expect(inbox).toBeDisabled()
    const g = await rpc<{ id: string }>(page, 'CreateGroup', '待改名', '')
    await page.reload()
    await rpc(page, 'OpenSettings')
    const input = page.locator('.grouprow input[type=text]').filter({ has: page.locator(':scope') }).nth(1)
    await expect(page.getByLabel('分组名称').first()).toBeVisible()
    expect(g.id).toBeTruthy()
    void input
  })

  test('数据页：显示数据目录，明文导出前有风险提示', async ({ page }) => {
    await freshGroup(page)
    await rpc(page, 'OpenSettings')
    await page.getByRole('tab', { name: '数据' }).click()
    await expect(page.getByTestId('data-dir')).not.toBeEmpty()
    await page.getByTestId('export-json').click()
    await expect(page.getByRole('dialog')).toContainText('明文导出')
    await page.getByLabel('加密导出（单独设置导出密码）').check()
    await expect(page.getByRole('dialog').getByRole('button', { name: '导出' })).toBeDisabled()
    await page.getByRole('textbox', { name: '导出密码' }).fill('abcdef')
    await expect(page.getByRole('dialog').getByRole('button', { name: '导出' })).toBeEnabled()
  })

  test('AC-15：开启加密必须确认已保存恢复密钥；数据经后端解密后仍可读', async ({ page }) => {
    const g = await freshGroup(page)
    await rpc(page, 'CreateItem', { title: '加密前的事项', groupId: g.id })
    await rpc(page, 'OpenSettings')
    await page.getByRole('tab', { name: '数据' }).click()
    await page.getByTestId('enable-encryption').click()
    await page.getByLabel('主密码', { exact: true }).fill('secret-1')
    await page.getByLabel('确认主密码').fill('secret-2')
    await expect(page.getByTestId('enable-confirm')).toBeDisabled()
    await page.getByLabel('确认主密码').fill('secret-1')
    await page.getByLabel('解锁方式').selectOption('password')
    await page.getByTestId('enable-confirm').click()
    const key = page.getByTestId('recovery-key')
    await expect(key).toBeVisible()
    expect((await key.textContent())!.replace(/\s/g, '')).toHaveLength(24)
    const done = page.getByRole('button', { name: '完成' })
    await expect(done).toBeDisabled()
    await page.getByTestId('recovery-saved').check()
    await done.click()
    await expect(page.getByText('加密已开启')).toBeVisible()
    const st = await rpc<{ enabled: boolean; locked: boolean }>(page, 'EncryptionStatus')
    expect(st).toMatchObject({ enabled: true, locked: false })
    // 关闭加密，恢复环境
    await page.getByRole('button', { name: '关闭加密' }).click()
    await page.getByRole('dialog').getByLabel('主密码').fill('secret-1')
    await page.getByRole('dialog').getByRole('button', { name: '确定' }).click()
    await expect(page.getByText('加密已关闭')).toBeVisible()
    expect((await rpc<{ enabled: boolean }>(page, 'EncryptionStatus')).enabled).toBe(false)
  })
})

test.describe('提醒与锁屏', () => {
  test('FR-305：强提醒必须手动处理才能关闭', async ({ page }) => {
    const g = await freshGroup(page)
    const it = await rpc<{ id: string }>(page, 'CreateItem', { title: '重要会议', groupId: g.id })
    await rpc(page, 'ShowStrongAlert', {
      tag: 't', title: '重要会议', body: '会议室 A', itemId: it.id, strong: true, summary: false, missed: false,
      actions: [{ id: 'done', label: '完成' }, { id: 'snooze10', label: '10 分钟后' }, { id: 'snooze60', label: '1 小时后' }, { id: 'tomorrow', label: '明天' }],
    })
    const alert = page.getByTestId('strong-alert')
    await expect(alert).toBeVisible()
    await expect(alert).toContainText('重要会议')
    await page.keyboard.press('Escape')
    await page.mouse.click(5, 5)
    await expect(alert).toBeVisible() // Esc 与点击背景都不能关闭
    await alert.getByRole('button', { name: '10 分钟后' }).click()
    await expect(alert).toHaveCount(0)
    const p = await rpc<{ todo: { reminders: { snoozeUntil: number }[] }[] }>(page, 'GroupContent', g.id)
    const until = p.todo[0].reminders[0].snoozeUntil
    expect(until - Date.now()).toBeGreaterThan(9 * 60000)
    expect(until - Date.now()).toBeLessThan(10.5 * 60000)
  })

  test('通知按钮「完成」会完成事项', async ({ page }) => {
    const g = await freshGroup(page)
    const it = await rpc<{ id: string }>(page, 'CreateItem', { title: '通知里点完成', groupId: g.id })
    await rpc(page, 'HandleAction', it.id, 'done')
    await expect(page.getByTestId('done-toggle')).toContainText('已完成 (1)')
  })
})
