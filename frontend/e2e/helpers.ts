import { expect, type Page } from '@playwright/test'

export async function rpc<T = unknown>(page: Page, method: string, ...args: unknown[]): Promise<T> {
  const res = await page.request.post(`/rpc/${method}`, { data: args })
  const body = await res.json()
  if (!res.ok()) throw new Error(body.error)
  return body.result as T
}

/** 每个用例使用独立分组，互不干扰；返回分组 id。 */
export async function freshGroup(page: Page, name = `测试${Date.now()}${Math.floor(Math.random() * 1000)}`) {
  // 开发服务器的叠加界面状态是全局的：先关掉上一个用例可能遗留的设置 / 快速输入
  await rpc(page, 'CloseSettings')
  await rpc(page, 'CloseQuick')
  const g = await rpc<{ id: string }>(page, 'CreateGroup', name, '#4a90d9')
  await rpc(page, 'ShowGroup', g.id)
  await page.goto('/')
  await expect(page.getByTestId('sticky')).toBeVisible()
  await expect(page.getByTestId('group-name')).toContainText(name)
  return { id: g.id, name }
}

export async function addViaInput(page: Page, text: string) {
  const input = page.getByTestId('add-input')
  await input.fill(text)
  await input.press('Enter')
}

export const rows = (page: Page) => page.getByTestId('item-row')
