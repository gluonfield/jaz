import { resolve } from 'node:path'
import tailwindcss from '@tailwindcss/vite'
import { tanstackRouter } from '@tanstack/router-plugin/vite'
import react from '@vitejs/plugin-react'
import { defineConfig } from 'electron-vite'
import { defineTelemetryEnv } from './vite.telemetry'

export default defineConfig(({ mode }) => ({
  main: {},
  preload: {},
  renderer: {
    define: defineTelemetryEnv(mode),
    resolve: {
      alias: {
        '@': resolve('src/renderer/src'),
      },
    },
    plugins: [
      // Must run before react().
      tanstackRouter({
        target: 'react',
        routesDirectory: resolve('src/renderer/src/routes'),
        generatedRouteTree: resolve('src/renderer/src/routeTree.gen.ts'),
      }),
      react(),
      tailwindcss(),
    ],
    server: {
      port: Number(process.env.ELECTRON_RENDERER_PORT ?? 5180),
      strictPort: true,
    },
  },
}))
