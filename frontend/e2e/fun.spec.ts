import { expect, test, type Page } from '@playwright/test'
import { freshGroup, rows, rpc } from './helpers'

const patchSettings = async (page: Page, patch: Record<string, unknown>) => {
  const s = await rpc<Record<string, unknown>>(page, 'GetSettings')
  return rpc(page, 'SaveSettings', { ...s, ...patch })
}
const devShell = async (page: Page) => (await page.request.get('/__dev/shell')).json() as Promise<{ opened: string }>
const rss = (title: string, items: string[]) =>
  `<?xml version="1.0" encoding="UTF-8"?><rss><channel><title>${title}</title>` +
  items.map((t, i) => `<item><title>${t}</title><link>https://example.com/${encodeURIComponent(t)}</link><guid>${t}</guid><pubDate>Sun, 15 Nov 2026 0${i}:00:00 +0800</pubDate></item>`).join('') +
  `</channel></rss>`

async function cleanupFun(page: Page) {
  if (await rpc<boolean>(page, 'BossMode')) await rpc(page, 'ToggleBossMode')
  for (const f of await rpc<{ id: string }[]>(page, 'ListFeeds')) await rpc(page, 'DeleteFeed', f.id)
  await patchSettings(page, { newsEnabled: false, petEnabled: false, showCountdown: false, bossGroupId: '' })
}

test.describe('摸鱼：RSS 新闻', () => {
  test.afterEach(async ({ page }) => cleanupFun(page))

  test('首次开启选择预置源；添加订阅、新闻条轮播、点击用浏览器打开、新闻列表、刷新与失败状态', async ({ page }) => {
    const name = `e2e-${Date.now()}.xml`
    const feedURL = `http://127.0.0.1:8790/__dev/feed/${name}`
    await page.request.post(`/__dev/feed/${name}`, { data: rss('测试源', ['旧闻一则', '摸鱼指南']) })
    await patchSettings(page, { newsSetupDone: false })
    await freshGroup(page)
    expect(await page.getByTestId('funbar').count()).toBe(0) // 默认关闭：便签上什么也没有

    await rpc(page, 'OpenSettings')
    await page.getByRole('tab', { name: '摸鱼' }).click()
    await page.getByLabel('启用新闻订阅（会联网）').click() // 先弹出选择框，确认后才真正开启
    // 首次开启：弹出预置源，默认勾选 3 个；这里全部取消，避免测试访问外网
    const list = page.getByTestId('preset-list')
    await expect(list.getByRole('checkbox')).toHaveCount(16)
    await expect(list).toContainText('长桥·金融快讯（待验证）') // 财经源默认不勾选
    await expect(list.getByRole('checkbox', { checked: true })).toHaveCount(3)
    for (const n of ['少数派', 'IT之家', '中国新闻网·滚动']) await list.getByLabel(n).uncheck()
    await page.getByTestId('preset-ok').click()
    await expect.poll(async () => (await rpc<{ newsEnabled: boolean }>(page, 'GetSettings')).newsEnabled).toBe(true)

    await page.getByTestId('feed-url').fill(feedURL)
    await page.getByTestId('feed-add').click()
    await expect(page.getByTestId('feed-row')).toHaveCount(1)
    await expect(page.getByTestId('feed-status')).toContainText('2 条')
    await expect(page.getByTestId('feed-row')).toContainText('测试源')

    // 失败的源：显示可读原因，不影响其他源
    await page.getByTestId('feed-url').fill('http://127.0.0.1:8790/__dev/feed/fail')
    await page.getByTestId('feed-add').click()
    await expect(page.getByTestId('feed-msg')).toContainText('503')
    await expect(page.getByTestId('feed-row').nth(1).getByTestId('feed-status')).toContainText('503')

    // 测试连接
    await page.getByTestId('feed-row').first().getByRole('button', { name: '测试' }).click()
    await expect(page.getByTestId('feed-test')).toContainText('连接正常：测试源，2 条')

    await page.getByTestId('settings-close').click()
    const ticker = page.getByTestId('news-title')
    await expect(ticker).toContainText('摸鱼指南') // 最新的一条
    await expect(ticker).toContainText('测试源')
    await ticker.click()
    await expect.poll(async () => (await devShell(page)).opened).toBe('https://example.com/' + encodeURIComponent('摸鱼指南'))

    // 新闻列表：未读数、全部已读
    await expect(page.getByTestId('news-open-list')).toHaveText('1')
    await page.getByTestId('news-open-list').click()
    await expect(page.getByTestId('news-row')).toHaveCount(2)
    await expect(page.getByTestId('group-name')).toContainText('新闻')
    await page.getByTestId('news-mark-all').click()
    await expect(page.getByTestId('news-open-list')).toHaveText('≡')

    // 源更新后「立即刷新」
    await page.request.post(`/__dev/feed/${name}`, { data: rss('测试源', ['旧闻一则', '摸鱼指南', '下午茶时间到']) })
    expect(await rpc<number>(page, 'RefreshNews')).toBe(1)
    await expect(page.getByTestId('news-row')).toHaveCount(3)
    await expect(page.getByTestId('news-open-list')).toHaveText('1')
  })
})

const devPost = (page: Page, data: Record<string, unknown>) => page.request.post('/__dev/shell', { data })
const dogEl = (page: Page) => page.locator('[data-testid=pet-dog] .dog')
/** 开启狗狗；named=false 时保留「第一次开启」状态（会弹起名框）。 */
async function enablePet(page: Page, named = true, extra: Record<string, unknown> = {}) {
  await rpc(page, 'ResetPet')
  if (named) await rpc(page, 'RenamePet', '博士')
  await patchSettings(page, { petEnabled: true, petBubbles: true, petWorry: true, petHoliday: true, petCorner: 'right', ...extra })
}

test.describe('摸鱼：戴眼镜的狗狗', () => {
  test.afterEach(async ({ page }) => {
    await devPost(page, { idle: 0 })
    await cleanupFun(page)
    await patchSettings(page, { petWater: false, petWaterNotify: false, petHoliday: true, petCorner: 'right' })
    await rpc(page, 'ResetPet'); await rpc(page, 'RenamePet', '博士')
  })

  test('第一次开启起名；单击摸头、双击握手、连点打喷嚏、眼睛跟着鼠标', async ({ page }) => {
    await freshGroup(page)
    await enablePet(page, false)
    // 起名框：预填「博士」，可一键选候选名
    await expect(page.getByTestId('name-input')).toHaveValue('博士')
    await page.getByRole('button', { name: '摸摸' }).click()
    await page.getByTestId('name-ok').click()
    await expect.poll(async () => rpc<{ name: string; named: boolean }>(page, 'GetPet')).toMatchObject({ name: '摸摸', named: true })
    await expect(page.getByTestId('name-input')).toHaveCount(0)

    const dog = page.getByTestId('pet-dog')
    await dog.click() // 单击：稍等一下才摸头（区分双击）
    await expect(page.locator('[data-fx=hearts]')).toHaveCount(1)
    await expect.poll(async () => (await rpc<{ pats: number }>(page, 'GetPet')).pats).toBe(1)

    await dog.dblclick() // 双击：握手，不算摸头
    await expect(dogEl(page)).toHaveAttribute('data-action', 'shake')
    await expect(page.getByTestId('pet-bubble')).toContainText('握手')
    await page.waitForTimeout(400)
    expect((await rpc<{ pats: number }>(page, 'GetPet')).pats).toBe(1)

    await page.waitForTimeout(2100) // 等两秒窗口过去，再连点 5 次
    for (let i = 0; i < 5; i++) await dog.click({ delay: 10 })
    await expect(dogEl(page)).toHaveAttribute('data-action', 'sneeze')
    await expect(page.getByTestId('pet-bubble')).toContainText('阿嚏')

    await expect(dogEl(page)).toHaveAttribute('data-action', '', { timeout: 5000 })
    await page.mouse.move(5, 200) // 鼠标在狗狗左边 → 眼珠往左
    await expect(page.locator('[data-testid=pet-dog] [data-look]')).toHaveAttribute('data-look', /^-/)
  })

  test('打盹时点一下会被叫醒；逾期担心；完成事项加经验；右键统计 / 改名 / 换位置 / 暂时收起', async ({ page }) => {
    await devPost(page, { idle: 4000 }) // 离开电脑一个多小时
    const g = await freshGroup(page)
    await enablePet(page)
    await expect(dogEl(page)).toHaveAttribute('data-mood', 'dozing')
    await page.getByTestId('pet-dog').click()
    await expect(dogEl(page)).toHaveAttribute('data-action', 'rub')
    await expect(page.getByTestId('pet-bubble')).toContainText('再睡五分钟')
    await expect(dogEl(page)).not.toHaveAttribute('data-mood', 'dozing')
    await devPost(page, { idle: 0 })

    await rpc(page, 'CreateItem', { title: '写周报', groupId: g.id })
    await rows(page).filter({ hasText: '写周报' }).getByRole('checkbox').click()
    await expect(page.locator('[data-fx=bone]')).toHaveCount(1)
    await expect.poll(async () => (await rpc<{ exp: number }>(page, 'GetPet')).exp).toBe(10)

    await rpc(page, 'CreateItem', { title: '早就该交的报告', groupId: g.id, dueAt: Date.now() - 3_600_000 })
    await expect(dogEl(page)).toHaveAttribute('data-mood', 'worried', { timeout: 8000 })
    await patchSettings(page, { petWorry: false })
    await expect(dogEl(page)).not.toHaveAttribute('data-mood', 'worried')

    const dog = page.getByTestId('pet-dog')
    await dog.click({ button: 'right' })
    await expect(page.getByTestId('pet-stats')).toContainText('经验 10 / 50')
    await expect(page.getByTestId('pet-stats')).toContainText('一起玩了 0 次球')
    await page.getByRole('menuitem', { name: '改名…' }).click()
    await page.getByTestId('name-input').fill('旺财')
    await page.getByTestId('name-ok').click()
    await expect.poll(async () => (await rpc<{ name: string }>(page, 'GetPet')).name).toBe('旺财')
    await dog.click({ button: 'right' })
    await page.getByRole('menuitem', { name: '换到左下角' }).click()
    await expect(page.getByTestId('pet')).toHaveClass(/left/)

    await dog.click({ button: 'right' })
    await page.getByRole('menuitem', { name: '暂时收起' }).click()
    await page.getByRole('dialog').getByRole('button', { name: '收起' }).click()
    await expect(page.getByTestId('pet')).toHaveCount(0)
    await page.reload()
    await expect(page.getByTestId('pet-dog')).toBeVisible()
  })

  test('按住拖到另一边；扔个球', async ({ page }) => {
    await freshGroup(page)
    await enablePet(page)
    const dog = page.getByTestId('pet-dog')
    const box = (await dog.boundingBox())!
    await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2)
    await page.mouse.down()
    await page.mouse.move(box.x - 60, box.y + box.height / 2, { steps: 5 })
    await page.mouse.move(40, box.y + box.height / 2, { steps: 5 })
    await page.mouse.up()
    await expect.poll(async () => (await rpc<{ petCorner: string }>(page, 'GetSettings')).petCorner).toBe('left')
    await expect(page.getByTestId('pet')).toHaveClass(/left/)
    await page.waitForTimeout(400)
    expect((await rpc<{ pats: number }>(page, 'GetPet')).pats).toBe(0) // 拖动不算摸头

    await dog.click({ button: 'right' })
    await page.getByRole('menuitem', { name: '扔个球' }).click()
    await expect(page.getByTestId('pet-ball')).toBeVisible()
    await expect(page.getByTestId('pet')).toHaveAttribute('data-play', 'run')
    await expect(dogEl(page)).toHaveAttribute('data-action', 'carry', { timeout: 3000 })
    await expect(page.getByTestId('pet-bubble')).toContainText('再来一次', { timeout: 4000 })
    await expect.poll(async () => (await rpc<{ plays: number }>(page, 'GetPet')).plays).toBe(1)
  })

  test('喝水提醒（含系统通知）、上班问候、节日装扮', async ({ page }) => {
    await page.clock.setFixedTime(new Date(2026, 11, 21, 10, 0)) // 星期一 10:00，离圣诞还有几天
    await freshGroup(page)
    await enablePet(page, true, { petWater: true, petWaterInterval: 45, petWaterNotify: true, workStart: '09:00', workEnd: '18:00' })
    const rem = page.getByTestId('pet-reminder')
    await expect(rem).toContainText('博士提醒你：喝口水吧')
    await expect.poll(async () => ((await page.request.get('/__dev/shell')).json() as Promise<{ notified: string }>).then((x) => x.notified)).toContain('喝口水吧')
    await expect(page.locator('[data-prop=bowl]')).toHaveCount(1)
    await page.getByTestId('reminder-done').click()
    await expect(rem).toHaveCount(0)
    await expect.poll(async () => (await rpc<{ waterToday: number }>(page, 'GetPet')).waterToday).toBe(1)

    // 节日：圣诞期间戴圣诞帽；关闭后恢复
    await page.clock.setFixedTime(new Date(2026, 11, 24, 9, 10))
    await page.reload()
    await expect(page.locator('[data-acc=santa]')).toHaveCount(1)
    await expect(page.getByTestId('pet-bubble')).toContainText('早！') // 平安夜是星期四：上班问候
    await patchSettings(page, { petHoliday: false })
    await expect(page.locator('[data-acc=santa]')).toHaveCount(0)
  })

  test('设置页：开启时起名（可用默认名）；提醒与节日选项；没解锁的装扮不能换', async ({ page }) => {
    await freshGroup(page)
    await rpc(page, 'ResetPet')
    await rpc(page, 'OpenSettings')
    await page.getByRole('tab', { name: '摸鱼' }).click()
    await page.getByLabel('养一只戴眼镜的狗狗').check()
    await expect(page.getByTestId('name-input')).toHaveValue('博士')
    await page.getByRole('button', { name: '就用默认的' }).click()
    await expect.poll(async () => rpc<{ name: string; named: boolean }>(page, 'GetPet')).toMatchObject({ name: '博士', named: true })
    await expect(page.getByTestId('pet-summary')).toContainText('Lv.1')
    await expect(page.getByLabel('节日自动装扮（春节、端午、中秋等）')).toBeChecked()
    await page.getByLabel('喝水 / 起身提醒').check()
    await expect(page.getByLabel('提醒间隔')).toHaveValue('45')
    await expect(page.getByLabel('同时弹系统通知（便签被挡住时也能看到）')).not.toBeChecked()
    await expect(rpc(page, 'SetPetLook', 'crown', 'round')).rejects.toThrow(/解锁/)
    await page.getByTestId('settings-close').click()
    await expect(page.getByTestId('pet-dog')).toBeVisible()
  })
})

test.describe('摸鱼：下班倒计时与正经模式', () => {
  test.afterEach(async ({ page }) => cleanupFun(page))

  test('倒计时显示；正经模式一键收起并切到指定分组，再按恢复', async ({ page }) => {
    const g = await freshGroup(page)
    const work = await rpc<{ id: string; name: string }>(page, 'CreateGroup', `工作${Date.now()}`, '')
    await patchSettings(page, { showCountdown: true, monthlySalary: 21750, petEnabled: true, bossGroupId: work.id })
    await expect(page.getByTestId('countdown')).toHaveText(/下班|上班|已下班|周末/)
    await expect(page.getByTestId('pet-dog')).toBeVisible()

    expect(await rpc<boolean>(page, 'ToggleBossMode')).toBe(true)
    await expect(page.getByTestId('funbar')).toHaveCount(0)
    await expect(page.getByTestId('pet')).toHaveCount(0)
    await expect(page.getByTestId('group-name')).toContainText(work.name)

    expect(await rpc<boolean>(page, 'ToggleBossMode')).toBe(false)
    await expect(page.getByTestId('countdown')).toBeVisible()
    await expect(page.getByTestId('pet-dog')).toBeVisible()
    await expect(page.getByTestId('group-name')).toContainText(g.name)
  })

  test('快捷键页有正经模式；摸鱼页说明', async ({ page }) => {
    await freshGroup(page)
    await rpc(page, 'OpenSettings')
    await page.getByRole('tab', { name: '快捷键' }).click()
    await expect(page.getByRole('button', { name: '正经模式（老板键）' })).toHaveText('Ctrl+Alt+H')
    await page.getByRole('tab', { name: '摸鱼' }).click()
    await expect(page.getByText('新闻条、狗狗、倒计时一键收起')).toBeVisible()
  })

  test('倒计时紧凑显示、金额悬停才显示、窄时自动分两行', async ({ page }) => {
    await page.clock.setFixedTime(new Date(2026, 10, 16, 13, 30)) // 星期一 13:30，已过一半
    await freshGroup(page)
    await patchSettings(page, { showCountdown: true, monthlySalary: 21750, workStart: '09:00', workEnd: '18:00', countdownMoney: 'hover', newsEnabled: true, newsSetupDone: true })
    const cd = page.getByTestId('countdown')
    await expect(cd).toHaveText('下班 4:30 · ¥ ••')
    await cd.hover()
    await expect(cd).toHaveText('距下班 4:30:00 · 今天已赚 ¥500.00')
    await page.getByTestId('titlebar').hover()
    await expect(cd).toHaveText('下班 4:30 · ¥ ••')
    // 默认宽度 300：倒计时与新闻分两行；拉宽后合回一行
    await expect(page.getByTestId('funbar')).toHaveAttribute('data-rows', '2')
    await page.setViewportSize({ width: 460, height: 420 })
    await expect(page.getByTestId('funbar')).toHaveAttribute('data-rows', '1')
    await page.setViewportSize({ width: 300, height: 420 })
    // 一直显示 / 不显示
    await patchSettings(page, { countdownMoney: 'always' })
    await expect(cd).toHaveText('下班 4:30 · ¥500.00')
    await patchSettings(page, { countdownMoney: 'never' })
    await expect(cd).toHaveText('下班 4:30')
    await patchSettings(page, { countdownMoney: 'hover', monthlySalary: 0, newsSetupDone: false })
  })
})
