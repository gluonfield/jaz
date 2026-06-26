import { resolve } from 'node:path'
import tailwindcss from '@tailwindcss/vite'
import { tanstackRouter } from '@tanstack/router-plugin/vite'
import react from '@vitejs/plugin-react'
import { defineConfig } from 'vite'
import { defineTelemetryEnv } from './vite.telemetry'

export default defineConfig(({ mode }) => ({
  root: resolve('src/renderer'),
  define: defineTelemetryEnv(mode),
  resolve: {
    alias: {
      '@': resolve('src/renderer/src'),
    },
  },
  plugins: [
    tanstackRouter({
      target: 'react',
      routesDirectory: resolve('src/renderer/src/routes'),
      generatedRouteTree: resolve('src/renderer/src/routeTree.gen.ts'),
    }),
    react(),
    tailwindcss(),
  ],
  build: {
    outDir: resolve('dist-web'),
    emptyOutDir: true,
  },
  server: {
    port: Number(process.env.WEB_RENDERER_PORT ?? 5181),
    strictPort: true,
    proxy: {
      '/health': 'http://127.0.0.1:5299',
      '/jazmem': 'http://127.0.0.1:5299',
      '/mcp': 'http://127.0.0.1:5299',
      '/v1': 'http://127.0.0.1:5299',
    },
  },
}))
