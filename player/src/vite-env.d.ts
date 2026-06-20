/// <reference types="vite/client" />

// File System Access API — Chromium ships this; TS lib doesn't yet.
// Used by useDirectoryLoader when available; we feature-detect at the call
// site and fall back to <input type=file webkitdirectory> on Safari/Firefox.
interface FileSystemDirectoryHandle {
  readonly kind: 'directory'
  readonly name: string
  values(): AsyncIterableIterator<FileSystemHandle>
  getFileHandle(name: string, options?: { create?: boolean }): Promise<FileSystemFileHandle>
}

interface FileSystemFileHandle {
  readonly kind: 'file'
  readonly name: string
  getFile(): Promise<File>
}

interface FileSystemHandle {
  readonly kind: 'file' | 'directory'
  readonly name: string
}

interface Window {
  showDirectoryPicker?: (options?: { mode?: 'read' | 'readwrite' }) => Promise<FileSystemDirectoryHandle>
}
