import path from 'node:path'
import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'

// 构建产物写入 Go 内嵌目录 internal/webui/dist，由同一个二进制在 /admin、/console 等路径提供。
// 资源走 /admin/assets/，文件名自带内容哈希，Go 侧对这类文件返回 immutable 长缓存。
// 注意不要给它们再追加 ?v=：分包会以 ./index-xxx.js 互相引用，URL 不一致会让入口模块执行两次。
export default defineConfig({
  base: '/admin/',
  plugins: [react(), tailwindcss()],
  resolve: { alias: { '@': path.resolve(import.meta.dirname, './src') } },
  build: {
    outDir: '../internal/webui/dist',
    emptyOutDir: true,
    assetsDir: 'assets',
    chunkSizeWarningLimit: 900,
  },
  server: {
    proxy: {
      '/admin/api': 'http://127.0.0.1:8800',
      '/api': 'http://127.0.0.1:8800',
    },
  },
})
