import react from '@vitejs/plugin-react'
import { defineConfig } from 'vitest/config'

// https://vite.dev/config/
export default defineConfig({
  plugins: [react()],
  server: {
    // Dev reverse proxy: the browser only talks to Vite, which forwards API
    // calls to Go. Same origin → no CORS. See knowledge-base/Notes.md, note 1.
    // ws: true lets the WebSocket Upgrade handshake through (Part 1.3).
    proxy: {
      '/api': 'http://localhost:8080',
      '/ws': { target: 'ws://localhost:8080', ws: true },
    },
  },
  test: {
    environment: 'jsdom',
    setupFiles: ['./src/test-setup.ts'],
  },
})
