import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

const component = readFileSync(new URL('./KnowledgeBase.vue', import.meta.url), 'utf8')

test('renders the document workspace instead of placing it in an inert template', () => {
  assert.doesNotMatch(component, /<template>\s*<div class="knowledge-main">/)
})
