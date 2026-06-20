import { useCallback, useEffect, useRef, useState } from 'react'
import {
  loadDirectory,
  type LoadInput,
  type LoadedDirectory,
} from '../lib/loadDirectory.ts'

// useDirectoryLoader wires the runtime directory picker (plan D2 + R2).
// Prefers the File System Access API (`window.showDirectoryPicker`) when
// the browser ships it (Chromium); falls back to `<input type=file
// webkitdirectory>` on Safari/Firefox. Both paths flow through
// loadDirectory.
//
// State machine:
//   idle → picking → loading → loaded
//                              ↘ error
//
// On replace, the PREVIOUS audioUrl (held in revokeRef) is revoked exactly
// before the new state is committed (R3 blob URL leak).

export interface DirectoryLoaderState {
  status: 'idle' | 'picking' | 'loading' | 'loaded' | 'error'
  data: LoadedDirectory | null
  error: string | null
}

export interface DirectoryLoaderApi extends DirectoryLoaderState {
  pickDirectory: () => Promise<void>
  loadFromFileList: (files: File[]) => Promise<void>
  supportsFsAccess: boolean
}

export function useDirectoryLoader(): DirectoryLoaderApi {
  const [state, setState] = useState<DirectoryLoaderState>({
    status: 'idle',
    data: null,
    error: null,
  })
  const revokeRef = useRef<string | null>(null)
  const supportsFsAccess =
    typeof window !== 'undefined' && typeof window.showDirectoryPicker === 'function'

  // Always revoke on unmount.
  useEffect(() => {
    return () => {
      if (revokeRef.current) {
        URL.revokeObjectURL(revokeRef.current)
        revokeRef.current = null
      }
    }
  }, [])

  const runLoad = useCallback(async (input: LoadInput) => {
    setState((s) => ({ ...s, status: 'loading', error: null }))
    try {
      const data = await loadDirectory(input)
      if (revokeRef.current) URL.revokeObjectURL(revokeRef.current)
      revokeRef.current = data.audioUrl
      setState({ status: 'loaded', data, error: null })
    } catch (e) {
      setState({ status: 'error', data: null, error: (e as Error).message })
    }
  }, [])

  const pickDirectory = useCallback(async () => {
    if (!supportsFsAccess || typeof window === 'undefined' || !window.showDirectoryPicker) {
      setState({
        status: 'error',
        data: null,
        error:
          'showDirectoryPicker is not available in this browser; use the directory-input fallback instead.',
      })
      return
    }
    setState((s) => ({ ...s, status: 'picking', error: null }))
    try {
      const handle = await window.showDirectoryPicker({ mode: 'read' })
      await runLoad({ kind: 'fs-handle', handle })
    } catch (e) {
      // AbortError = user cancelled the picker dialog. Treat as idle.
      if ((e as { name?: string }).name === 'AbortError') {
        setState({ status: 'idle', data: null, error: null })
        return
      }
      setState({ status: 'error', data: null, error: (e as Error).message })
    }
  }, [runLoad, supportsFsAccess])

  const loadFromFileList = useCallback(
    async (files: File[]) => {
      await runLoad({ kind: 'file-list', files })
    },
    [runLoad],
  )

  return { ...state, pickDirectory, loadFromFileList, supportsFsAccess }
}
