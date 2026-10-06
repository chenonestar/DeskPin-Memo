import { defineConfig } from '@playwright/test'

// 端到端测试：由 Go 开发服务器同时提供前端静态文件与 /rpc 接口（真实的业务层 + SQLite），
// 浏览器使用预装的 Chromium。运行：npm run build && npm run e2e
export default defineConfig({
  testDir: './e2e',
  timeout: 30_000,
  workers: 1,
  fullyParallel: false,
  reporter: [['list']],
  use: {
    baseURL: 'http://127.0.0.1:8790',
    viewport: { width: 300, height: 420 },
    launchOptions: { executablePath: process.env.CHROMIUM_PATH || undefined },
  },
  webServer: {
    // E2E_JITTER（如 80ms）给每个 RPC 加随机延迟，用来在本地放大时序问题
    command: `go run ../cmd/devserver -addr 127.0.0.1:8790 -static dist -jitter ${process.env.E2E_JITTER ?? '0s'}`,
    url: 'http://127.0.0.1:8790',
    reuseExistingServer: !process.env.CI,
    timeout: 120_000,
  },
})
