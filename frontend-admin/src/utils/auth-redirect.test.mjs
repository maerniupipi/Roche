import assert from 'node:assert/strict'
import test from 'node:test'
import { getLoginPath } from './auth-redirect.ts'

test('frontend-admin keeps login redirects under its deployment base', () => {
  assert.equal(getLoginPath('/admin/'), '/admin/login')
})

test('root deployments still redirect to the root login path', () => {
  assert.equal(getLoginPath('/'), '/login')
})
