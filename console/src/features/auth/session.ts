import { useSyncExternalStore } from 'react'

const storageKey = 'orag.console.session.v1'
const sessionEvent = 'orag:session-change'

export type ConsoleSession = {
  version: 1
  accessToken: string
  expiresAt: number
}

let currentSession = readStoredSession()

function readStoredSession(): ConsoleSession | null {
  if (typeof window === 'undefined') return null
  const raw = window.sessionStorage.getItem(storageKey)
  if (!raw) return null
  try {
    const value = JSON.parse(raw) as Partial<ConsoleSession>
    if (value.version !== 1 || typeof value.accessToken !== 'string' || !value.accessToken || typeof value.expiresAt !== 'number' || value.expiresAt <= Date.now()) {
      window.sessionStorage.removeItem(storageKey)
      return null
    }
    return value as ConsoleSession
  } catch {
    window.sessionStorage.removeItem(storageKey)
    return null
  }
}

function emitSessionChange() {
  window.dispatchEvent(new Event(sessionEvent))
}

export function storeSession(accessToken: string, expiresInSeconds: number) {
  currentSession = { version: 1, accessToken, expiresAt: Date.now() + Math.max(0, expiresInSeconds) * 1000 }
  window.sessionStorage.setItem(storageKey, JSON.stringify(currentSession))
  emitSessionChange()
}

export function clearSession() {
  currentSession = null
  window.sessionStorage.removeItem(storageKey)
  emitSessionChange()
}

export function getAccessToken() {
  if (currentSession && currentSession.expiresAt <= Date.now()) clearSession()
  return currentSession?.accessToken ?? null
}

function subscribe(callback: () => void) {
  const onStorage = (event: StorageEvent) => {
    if (event.key !== storageKey) return
    currentSession = readStoredSession()
    callback()
  }
  window.addEventListener(sessionEvent, callback)
  window.addEventListener('storage', onStorage)
  return () => {
    window.removeEventListener(sessionEvent, callback)
    window.removeEventListener('storage', onStorage)
  }
}

export function useSession() {
  return useSyncExternalStore(subscribe, () => currentSession, () => null)
}
