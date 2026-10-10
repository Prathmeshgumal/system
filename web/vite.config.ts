import tailwindcss from '@tailwindcss/vite'
import react from '@vitejs/plugin-react'
import { configDefaults, defineConfig } from 'vitest/config'

// https://vite.dev/config/
export default defineConfig({
  plugins: [react(), tailwindcss()],
  server: {
    // Dev reverse proxy: the browser only talks to Vite, which forwards API
    // calls to Go. Same origin → no CORS. See knowledge-base/Notes.md, note 1.
    // ws: true lets the WebSocket Upgrade handshake through (Part 1.3).
    //
    // changeOrigin: false keeps the browser's Host header (localhost:5173), like
    // Caddy does in full mode. Vite's shorthand '/api': 'http://…' quietly sets it
    // to true, rewriting Host to localhost:8080 — and then the API's Origin checks
    // (refresh/logout, WebSocket) see a cross-origin request and refuse it.
    proxy: {
      '/api': { target: 'http://localhost:8080', changeOrigin: false },
      '/ws': { target: 'ws://localhost:8080', ws: true, changeOrigin: false },
    },
  },
  test: {
    environment: 'jsdom',
    setupFiles: ['./src/test-setup.ts'],
    exclude: [...configDefaults.exclude, 'e2e/**'], // Playwright runs those
  },
})
