import assert from 'node:assert/strict'
import test from 'node:test'
import {
  buildReferenceSections,
  buildReferenceList,
  getDomainFromUrl,
  normalizeReferenceUrl,
} from './referenceSources.ts'

test('buildReferenceList separates tool and document references', () => {
  const items = buildReferenceList([
    {
      id: 'mcp-result-1',
      chunk_type: 'tool_result',
      knowledge_title: 'MCP Search',
      content: 'tool output',
      metadata: { source: 'MCP service' },
    },
    {
      id: 'chunk-1',
      knowledge_id: 'doc-1',
      knowledge_title: 'Policy',
      content: 'refund rules',
    },
  ])

  assert.equal(items.length, 2)
  assert.equal(items[0].kind, 'tool')
  assert.equal(items[1].kind, 'document')
})

test('buildReferenceList aggregates chunks from the same document', () => {
  const items = buildReferenceList([
    {
      id: 'chunk-1',
      knowledge_id: 'doc-1',
      knowledge_title: 'Policy',
      content: 'refund rules',
    },
    {
      id: 'chunk-2',
      knowledge_id: 'doc-1',
      knowledge_title: 'Policy',
      content: 'shipping rules',
    },
  ])

  assert.equal(items.length, 1)
  assert.equal(items[0].key, 'doc:doc-1')
  assert.deepEqual(items[0].chunkIds, ['chunk-1', 'chunk-2'])
  assert.match(items[0].content || '', /refund rules/)
  assert.match(items[0].content || '', /shipping rules/)
})

test('buildReferenceSections keeps tool results in their own section', () => {
  const sections = buildReferenceSections([
    {
      id: 'mcp-result-1',
      chunk_type: 'tool_result',
      knowledge_title: 'MCP Search',
      content: 'tool output',
      metadata: { source: 'MCP service' },
    },
  ])

  assert.equal(sections.length, 1)
  assert.equal(sections[0].id, 'tools')
  assert.equal(sections[0].items[0].kind, 'tool')
  assert.equal(sections[0].items[0].content, 'tool output')
})

test('normalizeReferenceUrl trims trailing slash', () => {
  assert.equal(
    normalizeReferenceUrl('https://example.com/path/'),
    'https://example.com/path',
  )
})

test('getDomainFromUrl strips www prefix', () => {
  assert.equal(getDomainFromUrl('https://www.example.com/x'), 'example.com')
})