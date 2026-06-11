/// <reference types="vitest/config" />
import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'

// GUI-specific vite config: same frontend but proxy to xbot serve backend.
// `make build` uses this config to produce dist/ for embedding.
export default defineConfig({
  plugins: [react(), tailwindcss()],
  // No dev server needed — Wails serves embedded assets.
  // For development, run `npm run dev` and set VITE_API_URL.
  server: {
    port: 5174,
    proxy: {
      '/api': {
        target: 'http://127.0.0.1:58080',
        changeOrigin: true,
      },
      '/ws': {
        target: 'ws://127.0.0.1:58080',
        ws: true,
        changeOrigin: true,
      },
    },
  },
  build: {
    outDir: 'dist',
    chunkSizeWarningLimit: 3000,
    rollupOptions: {
      output: {
        manualChunks(id) {
          if (id.includes('node_modules')) {
            if (id.includes('/react-dom/') || id.includes('/react/')) return 'vendor-react'
            if (id.includes('/@tiptap/') || id.includes('/tiptap-markdown/')) return 'vendor-tiptap'
            if (id.includes('/react-markdown/') || id.includes('/remark-gfm/')) return 'vendor-markdown'
            if (id.includes('/highlight.js/') || id.includes('/lowlight/')) return 'vendor-highlight'
            if (id.includes('/mermaid/')) return 'vendor-mermaid'
            if (id.includes('/katex/')) return 'vendor-katex'
          }
        },
      },
    },
  },
})
