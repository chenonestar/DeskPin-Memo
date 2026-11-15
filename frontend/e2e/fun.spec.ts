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
    await expect(list.getByRole('checkbox')).toHaveCount(12)
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

test.describe('摸鱼：戴眼镜的狗狗', () => {
  test.afterEach(async ({ page }) => { await cleanupFun(page); await rpc(page, 'ResetPet') })

  test('摸头、右键菜单、完成事项加经验、逾期担心、换位置、暂时收起', async ({ page }) => {
    const g = await freshGroup(page)
    expect(await page.getByTestId('pet').count()).toBe(0)
    await patchSettings(page, { petEnabled: true, petBubbles: true, petWorry: true })
    await rpc(page, 'ResetPet')
    const dog = page.getByTestId('pet-dog')
    await expect(dog).toBeVisible()
    await expect(page.locator('.dog')).toHaveAttribute('data-mood', 'normal')

    await dog.click() // 摸头
    await expect(page.locator('[data-fx=hearts]')).toHaveCount(1)
    await expect(page.locator('.dog')).toHaveAttribute('data-mood', 'happy')
    await expect.poll(async () => (await rpc<{ pats: number }>(page, 'GetPet')).pats).toBe(1)

    // 完成事项：叼走骨头，经验 +10
    await rpc(page, 'CreateItem', { title: '写周报', groupId: g.id })
    await rows(page).filter({ hasText: '写周报' }).getByRole('checkbox').click()
    await expect(page.locator('[data-fx=bone]')).toHaveCount(1)
    await expect.poll(async () => (await rpc<{ exp: number }>(page, 'GetPet')).exp).toBe(10)

    // 逾期：担心（眼镜滑到鼻尖）
    await rpc(page, 'CreateItem', { title: '早就该交的报告', groupId: g.id, dueAt: Date.now() - 3_600_000 })
    await expect(page.locator('.dog')).toHaveAttribute('data-mood', 'worried', { timeout: 8000 })
    await patchSettings(page, { petWorry: false })
    await expect(page.locator('.dog')).not.toHaveAttribute('data-mood', 'worried')

    // 右键菜单：统计、改名、换到左下角
    await dog.click({ button: 'right' })
    await expect(page.getByTestId('pet-stats')).toContainText('经验 10 / 50')
    await expect(page.getByTestId('pet-stats')).toContainText('帮你完成 1 件事')
    await page.getByRole('menuitem', { name: '改名…' }).click()
    await page.getByRole('dialog').getByRole('textbox').fill('旺财')
    await page.getByRole('dialog').getByRole('button', { name: '确定' }).click()
    await expect.poll(async () => (await rpc<{ name: string }>(page, 'GetPet')).name).toBe('旺财')
    await dog.click({ button: 'right' })
    await page.getByRole('menuitem', { name: '换到左下角' }).click()
    await expect(page.getByTestId('pet')).toHaveClass(/left/)

    // 暂时收起：直到下次启动（刷新页面模拟重启）
    await dog.click({ button: 'right' })
    await page.getByRole('menuitem', { name: '暂时收起' }).click()
    await page.getByRole('dialog').getByRole('button', { name: '收起' }).click()
    await expect(page.getByTestId('pet')).toHaveCount(0)
    await page.reload()
    await expect(page.getByTestId('pet-dog')).toBeVisible()
  })

  test('设置页：开关、统计、重置；没解锁的装扮不能换', async ({ page }) => {
    await freshGroup(page)
    await rpc(page, 'OpenSettings')
    await page.getByRole('tab', { name: '摸鱼' }).click()
    await page.getByLabel('养一只戴眼镜的狗狗').check()
    await expect(page.getByTestId('pet-summary')).toContainText('Lv.1')
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
    await expect(page.getByTestId('countdown')).toHaveText(/距下班|距上班|已下班|周末愉快/)
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
})
