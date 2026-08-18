export function getLoginPath(baseUrl: string): string {
  const segment = String(baseUrl || '/')
    .trim()
    .replace(/^\/+|\/+$/g, '')

  return segment ? `/${segment}/login` : '/login'
}
