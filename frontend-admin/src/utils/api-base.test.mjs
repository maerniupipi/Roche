import assert from 'node:assert/strict'
import test from 'node:test'
import { getApiBaseUrl } from './api-base.ts'

test('frontend-admin keeps API requests at the origin root', () => {
  assert.equal(getApiBaseUrl(), '/')
})
