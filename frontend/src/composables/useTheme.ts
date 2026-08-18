import { ref } from 'vue'
import {
  loadPreference,
  savePreference,
  migratePreferencesIntoUser,
} from './preferenceStorage'

export type ThemeMode = 'light' | 'dark' | 'system'

const THEME_KEY = 'theme'

function loadTheme(): ThemeMode {
  const value = loadPreference(THEME_KEY)
  if (value === 'light' || value === 'dark' || value === 'system') return value
  return 'light'
}

const currentTheme = ref<ThemeMode>(loadTheme())
let lastEffective: 'light' | 'dark' | null = null

function getSystemTheme(): 'light' | 'dark' {
  return window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light'
}

function syncDocumentTheme(effective: 'light' | 'dark') {
  const background = effective === 'dark' ? '#181818' : '#eeeeee'
  document.documentElement.style.background = background
  document.documentElement.style.minHeight = '100%'
  document.documentElement.style.colorScheme = effective
  if (document.body) {
    document.body.style.background = background
    document.body.style.minHeight = '100%'
  }
  const app = document.getElementById('app')
  if (app) {
    app.style.background = background
    app.style.minHeight = '100%'
  }
}

function applyTheme(mode: ThemeMode) {
  const effective = mode === 'system' ? getSystemTheme() : mode
  if (lastEffective === effective) return
  lastEffective = effective
  document.documentElement.setAttribute('theme-mode', effective)
  syncDocumentTheme(effective)
}

export function useTheme() {
  function setTheme(mode: ThemeMode): boolean {
    if (mode !== 'light' && mode !== 'dark' && mode !== 'system') return false
    currentTheme.value = mode
    savePreference(THEME_KEY, mode)
    applyTheme(mode)
    return true
  }

  return { currentTheme, setTheme }
}

export function initTheme() {
  currentTheme.value = loadTheme()
  applyTheme(currentTheme.value)

  window.matchMedia('(prefers-color-scheme: dark)').addEventListener('change', () => {
    if (currentTheme.value === 'system') {
      applyTheme('system')
    }
  })
}

export function reloadThemeFromStorage() {
  migratePreferencesIntoUser()
  currentTheme.value = loadTheme()
  applyTheme(currentTheme.value)
}
