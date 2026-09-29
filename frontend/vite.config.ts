import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// 开发时把 /rpc 与 /events 代理到 Go 开发服务器（go run ./cmd/devserver）
export default defineConfig({
  plugins: [react()],
  base: './',
  build: { outDir: 'dist', emptyOutDir: true, chunkSizeWarningLimit: 700 },
  server: {
    port: 5173,
    proxy: { '/rpc': 'http://127.0.0.1:8787', '/events': 'http://127.0.0.1:8787' },
  },
  test: { environment: 'node', include: ['src/**/*.test.ts'] },
})
