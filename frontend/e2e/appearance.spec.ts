import { expect, test, type Page } from '@playwright/test'
import { freshGroup, rpc } from './helpers'

const stickyBg = (page: Page) => page.getByTestId('sticky').evaluate((el) => getComputedStyle(el).backgroundColor)
const stickyOpacity = (page: Page) => page.getByTestId('sticky').evaluate((el) => getComputedStyle(el).opacity)
const devShell = async (page: Page) => (await page.request.get('/__dev/shell')).json() as Promise<{ native: boolean; opacity: number; background: string; radius: number }>
// 便签占满整个视口，无法用真实鼠标「移出去」；React 的 onMouseEnter/Leave 基于 mouseover/mouseout（relatedTarget 为空 = 进出页面）
const enter = (page: Page) => page.getByTestId('sticky').dispatchEvent('mouseover', { relatedTarget: null })
const leave = (page: Page) => page.getByTestId('sticky').dispatchEvent('mouseout', { relatedTarget: null })
const setNative = (page: Page, native: boolean) => page.request.post('/__dev/shell', { data: { native } })

async function setDefaults(page: Page, patch: Record<string, unknown>) {
  const s = await rpc<Record<string, unknown>>(page, 'GetSettings')
  await rpc(page, 'SaveSettings', { ...s, ...patch })
}

test.describe('外观：默认颜色 / 透明度 / 鼠标离开变淡', () => {
  test.afterEach(async ({ page }) => {
    await setNative(page, false)
    await setDefaults(page, { stickyColor: '#FFF3B0', opacity: 1, fadeOnLeave: false, fadedOpacity: 0.4 })
  })

  test('设置里改「默认便签颜色」，没单独设置过颜色的便签立刻跟随（含已保存过位置的便签）', async ({ page }) => {
    const g = await freshGroup(page)
    // 过去的 bug：便签第一次保存位置时把当时的默认色写死进窗口记录，之后改默认色就不再生效
    const other = await rpc<{ id: string }>(page, 'CreateGroup', `换组${Date.now()}`, '')
    await rpc(page, 'ShowGroup', other.id) // 切走会保存当前便签的窗口状态
    await rpc(page, 'ShowGroup', g.id)
    await page.reload()
    await expect(page.getByTestId('sticky')).toBeVisible()
    expect(await stickyBg(page)).toBe('rgb(255, 243, 176)') // #FFF3B0

    await rpc(page, 'OpenSettings')
    await page.getByRole('tab', { name: '外观' }).click()
    await page.getByRole('button', { name: '#D9F2C9' }).click()
    await expect.poll(async () => (await rpc<{ stickyColor: string }>(page, 'GetSettings')).stickyColor).toBe('#D9F2C9')
    await page.getByTestId('settings-close').click()
    await expect(page.getByTestId('sticky')).toBeVisible()
    await expect.poll(() => stickyBg(page)).toBe('rgb(217, 242, 201)') // 立刻变成新的默认色
    expect((await devShell(page)).background).toBe('#D9F2C9')            // 外壳的窗口底色也同步

    // 单独设置的颜色优先；之后再改默认色不覆盖它；「恢复默认外观」后重新跟随
    await page.getByTestId('menu-btn').click()
    await page.getByRole('button', { name: '#FFD9E4' }).click()
    await expect.poll(() => stickyBg(page)).toBe('rgb(255, 217, 228)')
    await setDefaults(page, { stickyColor: '#CFE8FF' })
    await page.reload()
    expect(await stickyBg(page)).toBe('rgb(255, 217, 228)')
    await page.getByTestId('menu-btn').click()
    await page.getByRole('menuitem', { name: /恢复默认外观/ }).click()
    await expect.poll(() => stickyBg(page)).toBe('rgb(207, 232, 255)')
    const w = await rpc<{ color: string; opacity: number }>(page, 'ApplyWindow')
    expect(w).toMatchObject({ color: '', opacity: 0 })
  })

  test('默认透明度同理：改默认值立刻生效，单独调整过的不被覆盖', async ({ page }) => {
    await freshGroup(page)
    await setDefaults(page, { opacity: 0.6 })
    await page.reload()
    expect(Number(await stickyOpacity(page))).toBeCloseTo(0.6, 2)
    await page.getByTestId('menu-btn').click()
    await page.getByRole('slider', { name: '透明度' }).fill('80')
    await expect.poll(async () => Number(await stickyOpacity(page))).toBeCloseTo(0.8, 2)
    await setDefaults(page, { opacity: 0.4 })
    await page.reload()
    expect(Number(await stickyOpacity(page))).toBeCloseTo(0.8, 2)
  })

  test('「鼠标离开后自动变淡」（浏览器预览走 CSS）：离开变淡，回来恢复', async ({ page }) => {
    await freshGroup(page)
    await setDefaults(page, { fadeOnLeave: true })
    await page.reload()
    const sticky = page.getByTestId('sticky')
    await sticky.hover()
    await expect.poll(async () => Number(await stickyOpacity(page))).toBe(1)
    await leave(page)
    await expect.poll(async () => Number(await stickyOpacity(page))).toBeLessThan(0.5)
    await enter(page)
    await expect.poll(async () => Number(await stickyOpacity(page))).toBe(1)
  })

  test('外壳处理窗口级透明度时：前端不叠加 CSS opacity（否则便签会叠在黑色窗口底色上变黑），变淡交给外壳', async ({ page }) => {
    await freshGroup(page)
    await setNative(page, true)
    await setDefaults(page, { fadeOnLeave: true, opacity: 0.8 })
    await page.reload()
    const sticky = page.getByTestId('sticky')
    await expect(sticky).toBeVisible()
    // CSS 始终不透明
    expect(Number(await stickyOpacity(page))).toBe(1)
    // （「启动时假定鼠标不在便签上、已处于变淡状态」由 Go 单测覆盖——开发服务器进程跨用例常驻，这里不依赖初始状态）
    // 离开 → 外壳变淡；进入 → 外壳恢复完整透明度（0.8）；再离开 → 再次变淡；这期间 CSS 一直是 1
    await leave(page)
    await expect.poll(async () => (await devShell(page)).opacity).toBeLessThan(0.8)
    await enter(page)
    await expect.poll(async () => (await devShell(page)).opacity).toBe(0.8)
    expect(Number(await stickyOpacity(page))).toBe(1)
    await leave(page)
    await expect.poll(async () => (await devShell(page)).opacity).toBeLessThan(0.8)
    expect(Number(await stickyOpacity(page))).toBe(1)
    // 变淡后仍不低于 25%
    expect((await devShell(page)).opacity).toBeGreaterThanOrEqual(0.25)
  })

  test('外壳处理透明度时，菜单里的透明度滑块直接作用到窗口', async ({ page }) => {
    await freshGroup(page)
    await setNative(page, true)
    await page.reload()
    await page.getByTestId('menu-btn').click()
    await page.getByRole('slider', { name: '透明度' }).fill('55')
    await expect.poll(async () => (await devShell(page)).opacity).toBeCloseTo(0.55, 2)
    expect(Number(await stickyOpacity(page))).toBe(1)
  })

  test('窗口圆角：便签 8，设置 / 快速输入叠加界面各自圆角并且保证不透明', async ({ page }) => {
    await freshGroup(page)
    await setNative(page, true)
    await rpc(page, 'ApplyWindow')
    expect((await devShell(page)).radius).toBe(8)
    await rpc(page, 'OpenSettings')
    expect(await devShell(page)).toMatchObject({ radius: 10, opacity: 1 })
    await rpc(page, 'CloseSettings')
    await rpc(page, 'OpenQuick')
    expect((await devShell(page)).radius).toBe(12)
    await rpc(page, 'CloseQuick')
    expect((await devShell(page)).radius).toBe(8)
  })
})

test.describe('鼠标穿透：Ctrl 临时恢复交互的可见反馈', () => {
  test('按住 Ctrl 时标题栏徽标显示「可操作」，松开后恢复「穿透」', async ({ page }) => {
    await freshGroup(page)
    await rpc(page, 'SetClickThrough', true)
    await page.reload()
    const badge = page.getByTestId('ct-badge')
    await expect(badge).toHaveText('穿透')
    await expect(badge).toHaveAttribute('data-live', 'false')
    // 外壳检测到 Ctrl 后会发出 window:interactive 事件
    await page.request.get('/__dev/emit?event=window:interactive&data=true')
    await expect(badge).toHaveText('可操作')
    await expect(badge).toHaveAttribute('data-live', 'true')
    await expect(badge).toHaveAttribute('title', /已按住 Ctrl/)
    await page.request.get('/__dev/emit?event=window:interactive&data=false')
    await expect(badge).toHaveText('穿透')
    // 关闭穿透时「可操作」状态一并清除
    await page.request.get('/__dev/emit?event=window:interactive&data=true')
    await rpc(page, 'SetClickThrough', false)
    await expect(badge).toHaveCount(0)
    await rpc(page, 'SetClickThrough', true)
    await expect(badge).toHaveText('穿透')
    await rpc(page, 'SetClickThrough', false)
  })

  test('「变淡后的透明度」只在勾选自动变淡时出现，并作为绝对值生效（CSS 预览）', async ({ page }) => {
    await freshGroup(page)
    await rpc(page, 'OpenSettings')
    await page.getByRole('tab', { name: '外观' }).click()
    await expect(page.getByRole('slider', { name: /变淡后的透明度/ })).toHaveCount(0)
    await page.getByLabel('鼠标离开后自动变淡').check()
    await page.getByRole('slider', { name: /变淡后的透明度/ }).fill('70')
    await expect.poll(async () => (await rpc<{ fadedOpacity: number }>(page, 'GetSettings')).fadedOpacity).toBeCloseTo(0.7, 2)
    await page.getByTestId('settings-close').click()
    await expect(page.getByTestId('sticky')).toBeVisible()
    await leave(page)
    await expect.poll(async () => Number(await stickyOpacity(page))).toBeCloseTo(0.7, 2)
    await setDefaults(page, { fadeOnLeave: false, fadedOpacity: 0.4 })
  })

  test('开启鼠标穿透后自动变淡不生效，并在设置页 / 菜单里明说；不改已保存的设置', async ({ page }) => {
    await freshGroup(page)
    await setDefaults(page, { fadeOnLeave: true, fadedOpacity: 0.4 })
    await page.reload()
    await leave(page)
    await expect.poll(async () => Number(await stickyOpacity(page))).toBeLessThan(0.5)
    await rpc(page, 'SetClickThrough', true)
    await expect.poll(async () => Number(await stickyOpacity(page))).toBe(1)
    await page.getByTestId('menu-btn').click()
    await expect(page.getByTestId('fade-ignored-hint')).toContainText('自动变淡未生效')
    await page.keyboard.press('Escape')
    await rpc(page, 'OpenSettings')
    await page.getByRole('tab', { name: '外观' }).click()
    await expect(page.getByTestId('fade-ignored-note')).toBeVisible()
    await expect(page.getByLabel('鼠标离开后自动变淡')).toBeEnabled()
    await expect(page.getByLabel('鼠标离开后自动变淡')).toBeChecked()
    await page.getByTestId('settings-close').click()
    await rpc(page, 'SetClickThrough', false)
    await expect(page.getByTestId('fade-ignored-note')).toHaveCount(0)
    expect((await rpc<{ fadeOnLeave: boolean }>(page, 'GetSettings')).fadeOnLeave).toBe(true)
    await setDefaults(page, { fadeOnLeave: false, fadedOpacity: 0.4 })
  })
})
