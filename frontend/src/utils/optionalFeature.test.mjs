import assert from 'node:assert/strict'
import test from 'node:test'
import { loadOptionalFeatureList } from './optionalFeature.ts'

test('loadOptionalFeatureList returns data from an available feature', async () => {
  const items = [{ id: 'mcp-1' }]

  assert.deepEqual(await loadOptionalFeatureList(async () => items), items)
})

test('loadOptionalFeatureList degrades an unavailable feature to an empty list', async () => {
  const loadUnavailableFeature = async () => {
    throw { status: 404, message: 'not found' }
  }

  assert.deepEqual(await loadOptionalFeatureList(loadUnavailableFeature), [])
})
