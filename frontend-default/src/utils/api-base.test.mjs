import assert from 'node:assert/strict'
import test from 'node:test'
import { getApiBaseUrl } from './api-base.ts'

test('frontend-default keeps API requests at the origin root', () => {
  assert.equal(getApiBaseUrl(), '/')
})
