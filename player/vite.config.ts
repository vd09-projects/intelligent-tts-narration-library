import { defineConfig } from 'vitest/config'
import react from '@vitejs/plugin-react'

// Vite + Vitest config for the reference player.
// - `plugins`: React JSX/HMR.
// - `test`: jsdom environment, jest-dom matchers via setup file.
//
// We import `defineConfig` from `vitest/config` (instead of `vite`) so the
// returned config type knows about the `test` key. The runtime behavior is
// identical to plain `vite` defineConfig.
export default defineConfig({
  plugins: [react()],
  test: {
    environment: 'jsdom',
    globals: true,
    setupFiles: ['./test/setup.ts'],
    include: ['test/**/*.test.{ts,tsx}'],
    css: false,
  },
})
