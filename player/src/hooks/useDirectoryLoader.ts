import { useCallback, useEffect, useRef, useState } from 'react'
import {
  loadDirectory,
  loadFromServerDir as loadFromServerDirLib,
  type LoadInput,
  type LoadedDirectory,
} from '../lib/loadDirectory.ts'
import { FIXTURE_BASE } from './useFixture.ts'

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
  // loadFromServerDir loads a whole plan by typed SERVER dir path (#70). The
  // hook builds the RefetchBaseInputs from the typed dir + server base URL.
  loadFromServerDir: (dir: string, serverBaseUrl: string) => Promise<void>
  // isLoading is true while ANY load (picker or load-by-path) is in flight, so
  // the UI can disable every trigger and enforce a single in-flight load (#70).
  isLoading: boolean
  supportsFsAccess: boolean
}

export function useDirectoryLoader(): DirectoryLoaderApi {
  const [state, setState] = useState<DirectoryLoaderState>({
    status: 'idle',
    data: null,
    error: null,
  })
  const [isLoading, setIsLoading] = useState(false)
  const revokeRef = useRef<string | null>(null)
  // Single-in-flight guard (#70 Decision v2 state-management): a synchronous ref
  // so a double-click / concurrent trigger is rejected BEFORE a second load can
  // race the shared revokeRef. The async `isLoading` state drives the UI; this
  // ref is the actual mutual-exclusion lock (state updates are async and would
  // let two clicks through in the same tick).
  const inFlightRef = useRef(false)
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

  // commitLoaded applies a successfully loaded directory with revoke-before-
  // replace discipline (R3): revoke the PREVIOUS audioUrl, then adopt the new
  // one. Shared by the picker paths and the load-by-path path so revokeRef is
  // mutated in exactly one place.
  const commitLoaded = useCallback((data: LoadedDirectory) => {
    if (revokeRef.current) URL.revokeObjectURL(revokeRef.current)
    revokeRef.current = data.audioUrl
    setState({ status: 'loaded', data, error: null })
  }, [])

  // withInFlight wraps a load so at most ONE runs at a time. A concurrent call
  // while one is in flight is dropped (returns immediately) — the click is
  // ignored, never raced. isLoading is cleared in finally on every exit path.
  const withInFlight = useCallback(async (run: () => Promise<void>) => {
    if (inFlightRef.current) return
    inFlightRef.current = true
    setIsLoading(true)
    try {
      await run()
    } finally {
      inFlightRef.current = false
      setIsLoading(false)
    }
  }, [])

  const runLoad = useCallback(
    async (input: LoadInput) => {
      setState((s) => ({ ...s, status: 'loading', error: null }))
      try {
        const data = await loadDirectory(input)
        commitLoaded(data)
      } catch (e) {
        // A rejected load must NOT update revokeRef to any uncommitted URL.
        setState({ status: 'error', data: null, error: (e as Error).message })
      }
    },
    [commitLoaded],
  )

  const pickDirectory = useCallback(
    () =>
      withInFlight(async () => {
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
      }),
    [runLoad, supportsFsAccess, withInFlight],
  )

  const loadFromFileList = useCallback(
    (files: File[]) => withInFlight(() => runLoad({ kind: 'file-list', files })),
    [runLoad, withInFlight],
  )

  // loadFromServerDir loads a whole plan by typed SERVER dir path (#70), via the
  // server's GET /artifact route. Shares the single-in-flight guard + revoke-
  // before-replace discipline with the picker paths.
  const loadFromServerDir = useCallback(
    (dir: string, serverBaseUrl: string) =>
      withInFlight(async () => {
        setState((s) => ({ ...s, status: 'loading', error: null }))
        try {
          const data = await loadFromServerDirLib(dir, {
            serverMode: true,
            dir,
            serverBaseUrl,
            fixtureBase: FIXTURE_BASE,
          })
          commitLoaded(data)
        } catch (e) {
          // A rejected load must NOT update revokeRef to any uncommitted URL.
          setState({ status: 'error', data: null, error: (e as Error).message })
        }
      }),
    [commitLoaded, withInFlight],
  )

  return {
    ...state,
    pickDirectory,
    loadFromFileList,
    loadFromServerDir,
    isLoading,
    supportsFsAccess,
  }
}
