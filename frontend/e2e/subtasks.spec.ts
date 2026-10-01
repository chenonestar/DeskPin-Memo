import { expect, test } from '@playwright/test'
import { addViaInput, freshGroup, rows, rpc } from './helpers'

const sub = (page: import('@playwright/test').Page) => page.getByTestId('subtask-row')

async function newItemWithSubs(page: import('@playwright/test').Page, title: string, subs: string[]) {
  const g = await freshGroup(page)
  const it = await rpc<{ id: string }>(page, 'CreateItem', { title, groupId: g.id, subtasks: subs.map((t) => ({ title: t })) })
  await page.reload()
  await expect(rows(page)).toHaveCount(1)
  return { g, it }
}

test.describe('FR-108 子任务', () => {
  test('悬停「＋」添加子任务，回车连续录入，显示进度 x/y', async ({ page }) => {
    await freshGroup(page)
    await addViaInput(page, '出差准备')
    const row = rows(page).first()
    await expect(row.getByTestId('subchip')).toHaveCount(0) // 没有子任务时没有进度标签
    await row.hover()
    await row.getByRole('button', { name: '添加子任务…' }).click()
    const add = page.getByTestId('subtask-add')
    await expect(add).toBeFocused()
    await add.fill('订机票')
    await add.press('Enter')
    await expect(sub(page)).toHaveCount(1)
    await expect(add).toBeFocused() // 连续录入：添加后焦点留在输入框
    await expect(add).toHaveValue('')
    await add.fill('订酒店')
    await add.press('Enter')
    await expect(sub(page)).toHaveCount(2)
    await expect(row.getByTestId('subchip')).toHaveText('☑ 0/2')
    await expect(sub(page).nth(0)).toContainText('订机票')
    await expect(sub(page).nth(1)).toContainText('订酒店')
  })

  test('勾选子任务更新进度；全部完成不会自动完成父事项', async ({ page }) => {
    await newItemWithSubs(page, '打包行李', ['衣服', '洗漱包'])
    const row = rows(page).first()
    const chip = row.getByTestId('subchip')
    await expect(chip).toHaveText('☑ 0/2')
    await expect(chip).toHaveAttribute('aria-expanded', 'false')
    await chip.click()
    await expect(chip).toHaveAttribute('aria-expanded', 'true')
    await sub(page).nth(0).getByRole('checkbox').click()
    await expect(chip).toHaveText('☑ 1/2')
    await expect(sub(page).nth(0)).toHaveClass(/done/)
    await sub(page).nth(1).getByRole('checkbox').click()
    await expect(chip).toHaveText('☑ 2/2')
    await expect(chip).toHaveClass(/all/)
    await expect(row).not.toHaveClass(/done/) // 父事项保持未完成
    // 再点标签收起
    await chip.click()
    await expect(page.getByTestId('subtasks')).toHaveCount(0)
    // 刷新后进度仍在
    await page.reload()
    await expect(rows(page).first().getByTestId('subchip')).toHaveText('☑ 2/2')
  })

  test('双击重命名：回车保存、Esc 取消', async ({ page }) => {
    await newItemWithSubs(page, '买菜', ['西红柿'])
    await rows(page).first().getByTestId('subchip').click()
    await sub(page).first().locator('.subtitle').dblclick()
    const edit = sub(page).first().getByLabel('子任务标题')
    await edit.fill('放弃的修改')
    await edit.press('Escape')
    await expect(sub(page).first()).toContainText('西红柿')
    await sub(page).first().locator('.subtitle').dblclick()
    await sub(page).first().getByLabel('子任务标题').fill('小番茄')
    await sub(page).first().getByLabel('子任务标题').press('Enter')
    await expect(sub(page).first()).toContainText('小番茄')
  })

  test('删除子任务可撤销（按钮和 Ctrl+Z）', async ({ page }) => {
    await newItemWithSubs(page, '大扫除', ['擦窗', '拖地'])
    const row = rows(page).first()
    await row.getByTestId('subchip').click()
    await sub(page).first().hover()
    await sub(page).first().getByRole('button', { name: '删除子任务' }).click()
    await expect(sub(page)).toHaveCount(1)
    await expect(row.getByTestId('subchip')).toHaveText('☑ 0/1')
    await page.getByTestId('snack').getByRole('button', { name: '撤销' }).click()
    await expect(sub(page)).toHaveCount(2)
    await expect(sub(page).first()).toContainText('擦窗') // 恢复到原位置

    // 勾选后 Ctrl+Z
    await sub(page).first().getByRole('checkbox').click()
    await expect(row.getByTestId('subchip')).toHaveText('☑ 1/2')
    await page.locator('.titlebar').click({ position: { x: 150, y: 14 } })
    await page.keyboard.press('Control+z')
    await expect(row.getByTestId('subchip')).toHaveText('☑ 0/2')
  })

  test('右键菜单「添加子任务…」展开并聚焦输入框', async ({ page }) => {
    await freshGroup(page)
    await addViaInput(page, '写周报')
    await rows(page).first().click({ button: 'right' })
    await page.getByRole('menuitem', { name: '添加子任务…' }).click()
    await expect(page.getByTestId('subtask-add')).toBeFocused()
    // 空面板可用 Esc 收起
    await page.keyboard.press('Escape')
    await expect(page.getByTestId('subtasks')).toHaveCount(0)
  })

  test('标题校验：空白不添加，超长由输入框限制', async ({ page }) => {
    await newItemWithSubs(page, '校验', [])
    const row = rows(page).first()
    await row.hover()
    await row.getByRole('button', { name: '添加子任务…' }).click()
    const add = page.getByTestId('subtask-add')
    await add.fill('   ')
    await add.press('Enter')
    await expect(sub(page)).toHaveCount(0)
    await expect(add).toHaveAttribute('maxlength', '200')
  })

  test('完成周期事项：下一次带上子任务且重置为未完成', async ({ page }) => {
    const g = await freshGroup(page)
    const now = Date.now()
    const it = await rpc<{ id: string }>(page, 'CreateItem', {
      title: '每日站会准备', groupId: g.id, dueAt: now - 3600_000,
      reminders: [{ remindAt: now - 3600_000, offsetMinutes: 0, repeatRule: 'FREQ=DAILY' }],
      subtasks: [{ title: '看板', done: true }, { title: '风险' }],
    })
    await page.reload()
    await expect(rows(page)).toHaveCount(1)
    await rows(page).first().getByRole('checkbox').click()
    await expect.poll(async () => {
      const gv = await rpc<{ todo: { id: string; subtasks: { title: string; done: boolean }[] }[] }>(page, 'GroupContent', g.id)
      const next = gv.todo.find((i) => i.id !== it.id)
      return next ? next.subtasks.map((s) => `${s.title}:${s.done}`).join('|') : ''
    }, { timeout: 8000 }).toBe('看板:false|风险:false')
  })

  test('搜索结果与智能视图中的事项同样显示进度', async ({ page }) => {
    const g = await freshGroup(page)
    const kw = `带子任务${Date.now()}`
    await rpc(page, 'CreateItem', { title: kw, groupId: g.id, subtasks: [{ title: 'a', done: true }, { title: 'b' }] })
    await page.reload()
    await expect(page.getByTestId('add-input')).toBeVisible()
    await page.keyboard.press('Control+f')
    await page.getByTestId('search-input').fill(kw)
    await expect(rows(page)).toHaveCount(1)
    await expect(rows(page).first().getByTestId('subchip')).toHaveText('☑ 1/2')
  })

  test('在子任务区点击或输入不会触发行拖拽 / 选中删除', async ({ page }) => {
    await newItemWithSubs(page, '拖拽安全', ['x'])
    await rows(page).first().locator('.title').click() // 先选中事项，Delete 键才可能误删它
    await rows(page).first().getByTestId('subchip').click()
    const add = page.getByTestId('subtask-add')
    await add.click()
    await add.fill('输入中按 Delete 不应删除事项')
    await add.press('Delete')
    await expect(rows(page)).toHaveCount(1)
  })
})
