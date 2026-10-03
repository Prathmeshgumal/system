import react from '@vitejs/plugin-react'
import { defineConfig } from 'vitest/config'

// https://vite.dev/config/
export default defineConfig({
  plugins: [react()],
  server: {
    // Dev reverse proxy: the browser only talks to Vite, which forwards API
    // calls to Go. Same origin → no CORS. See knowledge-base/notes/phase-0-notes.md, note 1.
    // /ws gets added here in Part 1.3.
    proxy: {
      '/api': 'http://localhost:8080',
    },
  },
  test: {
    environment: 'jsdom',
    setupFiles: ['./src/test-setup.ts'],
  },
})
