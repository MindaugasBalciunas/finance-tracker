/// <reference types="vitest/config" />
import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'

export default defineConfig({
  plugins: [react(), tailwindcss()],
  base: './',
  build: { chunkSizeWarningLimit: 700 },
  test: { environment: 'jsdom', globals: true, setupFiles: './src/test/setup.ts' },
  server: {
    port: 5175,
    proxy: { '/api': { target: process.env.API_URL || 'http://localhost:8080', changeOrigin: true } },
  },
})
