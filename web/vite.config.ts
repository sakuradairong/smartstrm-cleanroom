/// <reference types="vitest/config" />
import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

export default defineConfig({
  plugins: [react()],
  base: './',
  build: {
    outDir: '../internal/app/ui/dist',
    emptyOutDir: false,
  },
  server: {
    proxy: {
      '/api': 'http://localhost:8024',
      '/stream': 'http://localhost:8024',
      '/webhook': 'http://localhost:8024',
    },
  },
  test: {
    environment: 'jsdom',
    globals: true,
    setupFiles: './src/test/setup.ts',
    env: {
      // Keep date rendering assertions deterministic across developer and CI
      // machines while production continues to use the browser's local zone.
      TZ: 'UTC',
    },
    // Ant Design mounts substantial portal/style trees in jsdom. Full-suite
    // parallel runs need more than Vitest's 5s default on constrained CI.
    testTimeout: 15_000,
  },
})
