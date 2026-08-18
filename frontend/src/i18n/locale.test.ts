import assert from 'node:assert/strict'
import test from 'node:test'

import { loadSupportedLocale, resolveSupportedLocale } from './locale.ts'

test('resolveSupportedLocale keeps supported Chinese and English locales', () => {
  assert.equal(resolveSupportedLocale('zh-CN'), 'zh-CN')
  assert.equal(resolveSupportedLocale('en-US'), 'en-US')
})

test('resolveSupportedLocale resets removed or missing locales to Chinese', () => {
  assert.equal(resolveSupportedLocale('ko-KR'), 'zh-CN')
  assert.equal(resolveSupportedLocale('ru-RU'), 'zh-CN')
  assert.equal(resolveSupportedLocale(null), 'zh-CN')
})

test('loadSupportedLocale persists the fallback so later readers cannot restore a removed locale', () => {
  const values = new Map<string, string>([['locale', 'ko-KR']])
  const storage = {
    getItem(key: string) {
      return values.get(key) ?? null
    },
    setItem(key: string, value: string) {
      values.set(key, value)
    },
  }

  assert.equal(loadSupportedLocale(storage), 'zh-CN')
  assert.equal(values.get('locale'), 'zh-CN')
})
