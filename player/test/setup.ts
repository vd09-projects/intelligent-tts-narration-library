import '@testing-library/jest-dom/vitest'

// jsdom does not implement URL.createObjectURL / revokeObjectURL.
// Most tests stub these directly, but providing a default no-op keeps
// hooks that call into the URL global from blowing up under jsdom.
const U = URL as unknown as {
  createObjectURL?: (blob: Blob) => string
  revokeObjectURL?: (url: string) => void
}
if (typeof U.createObjectURL !== 'function') {
  U.createObjectURL = () => 'blob:test'
}
if (typeof U.revokeObjectURL !== 'function') {
  U.revokeObjectURL = () => {}
}
