import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

const component = readFileSync(new URL('./AgentEditorModal.vue', import.meta.url), 'utf8')

const localeSources = [
  '../../i18n/locales/zh-CN.ts',
  '../../i18n/locales/en-US.ts',
].map(path => readFileSync(new URL(path, import.meta.url), 'utf8'))

test('shows text_counter in the Skill tools group', () => {
  assert.match(
    component,
    /value:\s*'text_counter'[\s\S]*?label:\s*t\('agentEditor\.tools\.textCounter'\)[\s\S]*?group:\s*'skill'/,
  )
  assert.match(
    component,
    /key:\s*'skill'[\s\S]*?label:\s*t\('agentEditor\.tools\.groupSkill'\)/,
  )
  assert.match(component, /\.tool-group--skill\s+\.tool-group-bar/)
})

test('defines text counter and Skill tool group labels in every locale', () => {
  for (const locale of localeSources) {
    assert.match(locale, /\btextCounter:\s*['"]/)
    assert.match(locale, /\btextCounterDesc:\s*['"]/)
    assert.match(locale, /\bgroupSkill:\s*['"]/)
  }
})
