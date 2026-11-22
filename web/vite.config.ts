import { writeFileSync } from 'node:fs'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'
import { defineConfig } from 'vitest/config'

// The Go binary embeds web/dist, so the directory must exist (with its
// tracked .gitkeep) even after Vite empties it during a build.
const keepGitkeep = {
  name: 'keep-gitkeep',
  closeBundle() {
    writeFileSync('dist/.gitkeep', '')
  },
}

export default defineConfig({
  plugins: [react(), tailwindcss(), keepGitkeep],
  server: {
    port: 5173,
    // In development the API runs on :8080 (`make run-api`); set
    // NITHRO_DASHBOARD_URL=http://localhost:5173 so login returns here.
    proxy: {
      '/api': 'http://localhost:8080',
      '/auth': 'http://localhost:8080',
    },
  },
  build: { outDir: 'dist', emptyOutDir: true },
  test: {
    environment: 'jsdom',
    setupFiles: ['./src/test/setup.ts'],
  },
})
