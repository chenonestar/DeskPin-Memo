import { expect, test } from '@playwright/test'
import { addViaInput, freshGroup, rows, rpc } from './helpers'

test.describe('便签窗口', () => {
  test('AC-01：自然语言快速添加，带截止时间与提醒', async ({ page }) => {
    await freshGroup(page)
    await addViaInput(page, '明天下午3点 交报告')
    const row = rows(page).first()
    await expect(row).toContainText('交报告')
    await expect(row).toContainText('明天 15:00')
    await expect(row.getByTitle('已设置提醒')).toBeVisible()
    await expect(page.getByTestId('count')).toContainText('1 项未完成')
  })

  test('回车前预览解析结果', async ({ page }) => {
    await freshGroup(page)
    await page.getByTestId('add-input').fill('周五 买菜 #家')
    const pv = page.getByTestId('preview')
    await expect(pv).toBeVisible()
    await expect(pv).toContainText('#家')
    await expect(pv).toContainText('默认时刻')
  })

  test('AC-08：勾选后显示删除线，3 秒后进入已完成区，Ctrl+Z 撤销', async ({ page }) => {
    await freshGroup(page)
    await addViaInput(page, '写日报')
    const row = rows(page).first()
    await row.getByRole('checkbox').click()
    await expect(row).toHaveClass(/done/)
    await expect(page.getByTestId('snack')).toContainText('已完成')
    // 过渡期内仍在未完成列表
    await expect(page.getByTestId('todo-list').getByText('写日报')).toBeVisible()
    await expect(page.getByTestId('done-toggle')).toContainText('已完成 (1)', { timeout: 6000 })
    await expect(page.getByTestId('todo-list')).toHaveCount(0)
    // 展开已完成区
    await page.getByTestId('done-toggle').click()
    await expect(page.getByTestId('done-list').getByText('写日报')).toBeVisible()
    // Ctrl+Z 撤销
    await page.locator('body').click({ position: { x: 150, y: 200 } })
    await page.keyboard.press('Control+z')
    await expect(page.getByTestId('todo-list').getByText('写日报')).toBeVisible()
    await expect(page.getByTestId('todo-list').getByText('写日报')).not.toHaveCSS('text-decoration-line', 'line-through')
  })

  test('撤销按钮在完成后立即恢复', async ({ page }) => {
    await freshGroup(page)
    await addViaInput(page, '喝水')
    await rows(page).first().getByRole('checkbox').click()
    await page.getByTestId('snack').getByRole('button', { name: '撤销' }).click()
    await expect(rows(page).first()).not.toHaveClass(/done/)
    await expect(page.getByTestId('count')).toContainText('1 项未完成')
  })

  test('双击行内编辑：回车保存、Esc 取消', async ({ page }) => {
    await freshGroup(page)
    await addViaInput(page, '旧标题')
    await rows(page).first().locator('.title').dblclick()
    const edit = page.locator('input.edit')
    await edit.fill('放弃的修改')
    await edit.press('Escape')
    await expect(rows(page).first()).toContainText('旧标题')
    await rows(page).first().locator('.title').dblclick()
    await page.locator('input.edit').fill('新标题')
    await page.locator('input.edit').press('Enter')
    await expect(rows(page).first()).toContainText('新标题')
  })

  test('AC-09：删除进入回收站，可恢复且字段完整', async ({ page }) => {
    await freshGroup(page)
    await addViaInput(page, '明天 09:00 报销 #财务 !高')
    const inTrash = rows(page).filter({ hasText: '报销' }) // 回收站是全局视图，只看本用例的事项
    const row = rows(page).first()
    await expect(row).toHaveClass(/prio-2/)
    await row.click({ button: 'right' })
    await page.getByRole('menuitem', { name: '删除' }).click()
    await expect(rows(page)).toHaveCount(0)
    await expect(page.getByTestId('snack')).toContainText('已删除')
    // 回收站
    await page.getByTestId('group-name').click()
    await page.getByRole('menuitem', { name: '回收站' }).click()
    await expect(page.getByText('删除的事项保留 30 天后自动清除')).toBeVisible()
    await expect(inTrash.first()).toContainText('报销')
    await inTrash.first().getByRole('button', { name: '恢复' }).click()
    await expect(inTrash).toHaveCount(0)
    // 回到分组：字段完整
    await page.keyboard.press('Escape')
    await expect(rows(page).first()).toContainText('报销')
    await expect(rows(page).first()).toContainText('#财务')
    await expect(rows(page).first()).toContainText('明天 09:00')
    await expect(rows(page).first()).toHaveClass(/prio-2/)
  })

  test('清空回收站需要二次确认', async ({ page }) => {
    await freshGroup(page)
    const tmp = `临时${Date.now()}`
    await addViaInput(page, tmp)
    await rows(page).first().click({ button: 'right' })
    await page.getByRole('menuitem', { name: '删除' }).click()
    await page.getByTestId('group-name').click()
    await page.getByRole('menuitem', { name: '回收站' }).click()
    await page.getByRole('button', { name: '清空回收站' }).click()
    await expect(page.getByRole('dialog')).toContainText('清空后无法恢复')
    await page.getByRole('dialog').getByRole('button', { name: '取消' }).click()
    await expect(rows(page).filter({ hasText: tmp })).toHaveCount(1)
    await page.getByRole('button', { name: '清空回收站' }).click()
    await page.getByRole('dialog').getByRole('button', { name: '确定' }).click()
    await expect(rows(page)).toHaveCount(0)
  })

  test('FR-106：逾期置顶标红，其后按今天、有日期、无日期', async ({ page }) => {
    const g = await freshGroup(page)
    const now = Date.now()
    await rpc(page, 'CreateItem', { title: '无日期', groupId: g.id })
    await rpc(page, 'CreateItem', { title: '五天后', groupId: g.id, dueAt: now + 5 * 86400000 })
    await rpc(page, 'CreateItem', { title: '逾期的', groupId: g.id, dueAt: now - 3 * 3600000 })
    await page.reload()
    await expect(rows(page)).toHaveCount(3)
    const titles = await rows(page).locator('.title').allTextContents()
    expect(titles).toEqual(['逾期的', '五天后', '无日期'])
    await expect(rows(page).first()).toHaveClass(/overdue/)
    await expect(page.getByTestId('count')).toContainText('1 逾期')
    await expect(rows(page).first().locator('.due')).toHaveCSS('color', 'rgb(217, 48, 37)')
  })

  test('右键菜单：优先级与移动到分组', async ({ page }) => {
    const g = await freshGroup(page)
    const otherName = `目标${Date.now()}`
    const other = await rpc<{ id: string }>(page, 'CreateGroup', otherName, '')
    await addViaInput(page, '搬家')
    await rows(page).first().click({ button: 'right' })
    await page.getByRole('menuitem', { name: '设置优先级' }).click()
    await page.getByRole('menuitem', { name: '高' }).click()
    await expect(rows(page).first()).toHaveClass(/prio-2/)
    await rows(page).first().click({ button: 'right' })
    await page.getByRole('menuitem', { name: '移动到分组' }).click()
    await page.locator('.menu .indent').getByRole('menuitem', { name: otherName }).click()
    await expect(rows(page)).toHaveCount(0)
    const gv = await rpc<{ todo: { title: string }[] }>(page, 'GroupContent', other.id)
    expect(gv.todo.map((i) => i.title)).toEqual(['搬家'])
    expect(g.id).not.toBe(other.id)
  })

  test('编辑对话框：设置截止时间与每周重复提醒', async ({ page }) => {
    const g = await freshGroup(page)
    await addViaInput(page, '周会准备')
    await rows(page).first().click({ button: 'right' })
    await page.getByRole('menuitem', { name: '设置提醒…' }).click()
    const dlg = page.getByRole('dialog')
    await dlg.getByLabel('截止时间').fill('2030-01-07T09:00')
    await dlg.getByLabel('提醒', { exact: true }).selectOption('b15')
    await dlg.getByLabel('重复').selectOption('weekly')
    await dlg.getByLabel('星期').selectOption('1')
    await dlg.getByRole('button', { name: '保存' }).click()
    await expect(dlg).toHaveCount(0)
    await expect(rows(page).first()).toContainText('每周一')
    const gv = await rpc<{ todo: { reminders: { offsetMinutes: number; repeatRule: string; remindAt: number }[]; dueAt: number }[] }>(page, 'GroupContent', g.id)
    const r = gv.todo[0].reminders[0]
    expect(r.offsetMinutes).toBe(15)
    expect(r.repeatRule).toBe('FREQ=WEEKLY;BYDAY=MO')
    expect(r.remindAt).toBe(gv.todo[0].dueAt - 15 * 60000)
  })

  test('「截止前提醒」没有截止时间时给出错误提示', async ({ page }) => {
    await freshGroup(page)
    await addViaInput(page, '无日期事项')
    await rows(page).first().click({ button: 'right' })
    await page.getByRole('menuitem', { name: '设置提醒…' }).click()
    const dlg = page.getByRole('dialog')
    await dlg.getByLabel('提醒', { exact: true }).selectOption('b30')
    await dlg.getByRole('button', { name: '保存' }).click()
    await expect(dlg.getByRole('alert')).toContainText('需要先设置截止时间')
  })

  test('FR-404：搜索高亮关键字，包含已完成事项', async ({ page }) => {
    const g = await freshGroup(page)
    const kw = `邮件${Date.now()}`
    const it = await rpc<{ id: string }>(page, 'CreateItem', { title: `回复 HR ${kw}`, groupId: g.id })
    await rpc(page, 'CreateItem', { title: '买菜', groupId: g.id })
    await rpc(page, 'Toggle', it.id, true)
    await page.reload()
    await expect(page.getByTestId('add-input')).toBeVisible()
    await page.keyboard.press('Control+f')
    const input = page.getByTestId('search-input')
    await expect(input).toBeFocused()
    await input.fill(kw)
    await expect(rows(page)).toHaveCount(1)
    await expect(rows(page).first().locator('mark')).toHaveText(kw)
    await input.fill('')
    await expect(rows(page)).toHaveCount(0)
  })

  test('智能视图：今天 / 逾期 / 无日期', async ({ page }) => {
    const g = await freshGroup(page)
    const title = `孤立事项${Date.now()}`
    await rpc(page, 'CreateItem', { title, groupId: g.id })
    await page.reload()
    await page.getByTestId('group-name').click()
    await page.getByRole('menuitem', { name: '无日期' }).click()
    await expect(page.getByTestId('flat-list').getByText(title)).toBeVisible()
    await expect(page.getByTestId('group-name')).toContainText('无日期')
  })

  test('折叠与锁定通过后端保存', async ({ page }) => {
    const g = await freshGroup(page)
    await page.getByTestId('titlebar').dblclick({ position: { x: 150, y: 14 } })
    await expect(page.getByTestId('todo-list')).toHaveCount(0)
    await expect(page.getByTestId('add-input')).toHaveCount(0)
    let w = await rpc<{ collapsed: boolean }>(page, 'ApplyWindow')
    expect(w.collapsed).toBe(true)
    await page.getByTestId('titlebar').dblclick({ position: { x: 150, y: 14 } })
    await expect(page.getByTestId('add-input')).toBeVisible()
    await page.getByTestId('menu-btn').click()
    await page.getByRole('menuitem', { name: '锁定位置' }).click()
    w = await rpc<{ collapsed: boolean; locked: boolean }>(page, 'ApplyWindow') as never
    expect((w as unknown as { locked: boolean }).locked).toBe(true)
    await expect(page.locator('.grip')).toHaveCount(0)
    expect(g.id).toBeTruthy()
  })

  test('FR-202：窗口模式三选一', async ({ page }) => {
    await freshGroup(page)
    await page.getByTestId('mode-btn').click()
    await page.getByRole('menuitem', { name: '置顶' }).click()
    let w = await rpc<{ mode: string }>(page, 'ApplyWindow')
    expect(w.mode).toBe('top')
    await page.getByTestId('mode-btn').click()
    await page.getByRole('menuitem', { name: '钉在桌面' }).click()
    w = await rpc<{ mode: string }>(page, 'ApplyWindow')
    expect(w.mode).toBe('desktop')
  })

  test('FR-106：拖拽手动排序', async ({ page }) => {
    const g = await freshGroup(page)
    for (const t of ['甲', '乙', '丙']) await rpc(page, 'CreateItem', { title: t, groupId: g.id })
    await page.reload()
    await expect(rows(page)).toHaveCount(3)
    const first = rows(page).nth(0)
    const third = rows(page).nth(2)
    const a = (await third.boundingBox())!
    const b = (await first.boundingBox())!
    await page.mouse.move(a.x + 100, a.y + a.height / 2)
    await page.mouse.down()
    await page.mouse.move(a.x + 100, a.y + a.height / 2 - 10, { steps: 4 })
    await page.mouse.move(b.x + 100, b.y + 4, { steps: 10 })
    await page.mouse.up()
    await expect.poll(async () => (await rows(page).locator('.title').allTextContents()).join('')).toBe('丙甲乙')
    await page.reload()
    await expect(rows(page)).toHaveCount(3)
    expect((await rows(page).locator('.title').allTextContents()).join('')).toBe('丙甲乙')
  })

  test('Delete 键删除选中的事项', async ({ page }) => {
    await freshGroup(page)
    await addViaInput(page, '选中后删除')
    await rows(page).first().click()
    await page.keyboard.press('Delete')
    await expect(rows(page)).toHaveCount(0)
  })

  test('暗色主题跟随设置', async ({ page }) => {
    await freshGroup(page)
    const s = await rpc<Record<string, unknown>>(page, 'GetSettings')
    await rpc(page, 'SaveSettings', { ...s, theme: 'dark' })
    await page.reload()
    await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark')
    await rpc(page, 'SaveSettings', { ...s, theme: 'system' })
  })
})
