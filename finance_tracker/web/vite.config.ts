import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

export default defineConfig({
  plugins: [react()],
  base: './',
  build: { rollupOptions: { output: { manualChunks: { recharts: ['recharts'], markdown: ['react-markdown', 'remark-gfm'] } } } },
  server: {
    port: 5175,
    proxy: { '/api': { target: process.env.API_URL || 'http://localhost:8091', changeOrigin: true } },
  },
})
