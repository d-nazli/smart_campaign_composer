import vue from '@vitejs/plugin-vue'
import { defineConfig } from 'vitest/config'

// In docker-compose.dev.yml API_PROXY_TARGET is http://api:8080.
// When running Vite on the host, it falls back to the published API port.
const apiTarget = process.env.API_PROXY_TARGET ?? 'http://localhost:8081'

export default defineConfig({
  plugins: [vue()],
  server: {
    host: true,
    port: 5173,
    proxy: {
      '/api': {
        target: apiTarget,
        changeOrigin: true,
      },
    },
  },
  test: {
    // Plain logic runs in Node; a component test opts in with `// @vitest-environment jsdom`.
    environment: 'node',
    setupFiles: ['./src/test/setup.ts'],
  },
})
