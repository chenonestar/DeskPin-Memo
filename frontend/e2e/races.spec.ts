import { expect, test } from '@playwright/test'
import { freshGroup, rpc } from './helpers'

// 界面连续快速修改时，保存请求可能乱序到达后端（Wails 每次调用各自独立处理，网络 / 调度也会抖动）。
// 这里用请求拦截让「第一个请求」故意晚到，确认后一个修改不会被先发出、后到达的旧值覆盖。
test.describe('请求乱序下的状态一致性', () => {
  test('设置页：连续两次修改，第一个保存请求晚到也不会覆盖第二个', async ({ page }) => {
    await freshGroup(page)
    await rpc(page, 'OpenSettings')
    await page.getByRole('tab', { name: '提醒' }).click()

    let n = 0
    await page.route('**/rpc/SaveSettings', async (route) => {
      n++
      if (n === 1) await new Promise((r) => setTimeout(r, 600)) // 第一个保存请求被拖延
      await route.continue()
    })
    await page.getByLabel('启用免打扰时段（提醒延后到时段结束）').check()
    await page.getByRole('textbox', { name: '开始' }).fill('23:30')
    await page.getByRole('textbox', { name: '结束' }).fill('07:15')

    // 等被拖延的第一个请求也到达后端之后再检查：此时若旧请求覆盖了新值就会暴露
    await page.waitForTimeout(1200)
    // 最终后端状态必须是「最后一次修改」，而不是被先发出的旧请求覆盖
    expect((await rpc<{ dnd: { enabled: boolean; start: string; end: string } }>(page, 'GetSettings')).dnd)
      .toMatchObject({ enabled: true, start: '23:30', end: '07:15' })
    // 界面也停在最新值上（旧请求的响应不能把界面改回去）
    await expect(page.getByRole('textbox', { name: '开始' })).toHaveValue('23:30')
    await expect(page.getByRole('textbox', { name: '结束' })).toHaveValue('07:15')
    expect(n).toBeGreaterThanOrEqual(3)
    await page.unroute('**/rpc/SaveSettings')
    await page.getByLabel('启用免打扰时段（提醒延后到时段结束）').uncheck()
  })

  test('透明度滑块连续拖动：最终保存的是最后一次的值', async ({ page }) => {
    await freshGroup(page)
    let n = 0
    await page.route('**/rpc/SetWindowOpacity', async (route) => {
      n++
      if (n === 1) await new Promise((r) => setTimeout(r, 500))
      await route.continue()
    })
    await page.getByTestId('menu-btn').click()
    const slider = page.getByRole('slider', { name: '透明度' })
    await slider.fill('80')
    await slider.fill('55')
    await page.waitForTimeout(1200) // 等被拖延的第一个请求也到达
    expect((await rpc<{ opacity: number }>(page, 'ApplyWindow')).opacity).toBeCloseTo(0.55, 2)
    await page.unroute('**/rpc/SetWindowOpacity')
  })
})
