import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

// Wails dev 通过 frontend:dev:watcher 启动本 dev server，并按端口代理到 WebView。
export default defineConfig({
  plugins: [vue()],
  clearScreen: false,
  server: {
    port: 5173,
    strictPort: true,
  },
  build: {
    outDir: 'dist',
    emptyOutDir: true,
  },
})
