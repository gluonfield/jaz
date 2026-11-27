import { resolve } from 'node:path'
import tailwindcss from '@tailwindcss/vite'
import { tanstackRouter } from '@tanstack/router-plugin/vite'
import react from '@vitejs/plugin-react'
import { defineConfig } from 'electron-vite'
import { defineCloudEnv } from './vite.cloud'
import { pdfjsData } from './vite.pdfjs'
import { defineTelemetryEnv } from './vite.telemetry'

export default defineConfig(({ mode }) => ({
  main: {
    define: defineCloudEnv(mode),
    resolve: { alias: { '@shared': resolve('src/shared'), '@main': resolve('src/main') } },
  },
  preload: {
    define: defineCloudEnv(mode),
    resolve: { alias: { '@shared': resolve('src/shared'), '@preload': resolve('src/preload') } },
    build: { lib: { entry: { index: resolve('src/preload/index.ts'), chrome: resolve('src/preload/chrome.ts'), passkeyFallback: resolve('src/preload/passkeyFallback.ts'), browserPasswordsFrame: resolve('src/preload/browserPasswordsFrame.ts') } } },
  },
  renderer: {
    define: { ...defineTelemetryEnv(mode), ...defineCloudEnv(mode) },
    resolve: {
      alias: {
        '@shared': resolve('src/shared'),
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
      pdfjsData(),
    ],
    server: {
      port: Number(process.env.ELECTRON_RENDERER_PORT ?? 5180),
      strictPort: true,
    },
  },
}))
