export type SSOProvider = 'saml'

export interface SSOCallback {
  provider: SSOProvider
  result: string | null
  error: string | null
  errorDescription: string | null
}

function frontendBasePath(): string {
  const base = import.meta.env.BASE_URL || '/'
  return base.startsWith('/') ? base : `/${base}`
}

export function getFrontendSSOCallbackURL(): string {
  return new URL(frontendBasePath(), window.location.origin).toString()
}

export function readSSOCallback(): SSOCallback | null {
  const rawHash = window.location.hash.startsWith('#')
    ? window.location.hash.slice(1)
    : window.location.hash
  if (!rawHash) return null

  const params = new URLSearchParams(rawHash)
  for (const provider of ['saml'] as const) {
    const result = params.get(`${provider}_result`)
    const error = params.get(`${provider}_error`)
    if (result || error) {
      return {
        provider,
        result,
        error,
        errorDescription: params.get(`${provider}_error_description`),
      }
    }
  }
  return null
}

export function hasPendingSSOCallback(): boolean {
  return typeof window !== 'undefined' && readSSOCallback() !== null
}

export function decodeSSOResult(encoded: string): any {
  const normalized = encoded.replace(/-/g, '+').replace(/_/g, '/')
  const padded = normalized + '='.repeat((4 - (normalized.length % 4)) % 4)
  const binary = window.atob(padded)
  const bytes = Uint8Array.from(binary, (char) => char.charCodeAt(0))
  return JSON.parse(new TextDecoder().decode(bytes))
}

export function clearSSOCallbackFragment(): void {
  window.history.replaceState({}, document.title, window.location.pathname + window.location.search)
}
