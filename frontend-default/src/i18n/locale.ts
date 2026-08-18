export type SupportedLocale = 'zh-CN' | 'en-US'

type LocaleStorage = {
  getItem(key: string): string | null
  setItem(key: string, value: string): void
}

export function resolveSupportedLocale(locale: string | null): SupportedLocale {
  if (locale === 'en-US') {
    return 'en-US'
  }
  return 'zh-CN'
}

export function loadSupportedLocale(storage: LocaleStorage): SupportedLocale {
  const storedLocale = storage.getItem('locale')
  const locale = resolveSupportedLocale(storedLocale)
  if (storedLocale !== locale) {
    storage.setItem('locale', locale)
  }
  return locale
}
