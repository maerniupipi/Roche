import assert from 'node:assert/strict'
import test from 'node:test'

import {
  buildCitationEnrichedCopyText,
  type CitationKnowledgeRef,
} from '../src/utils/citationMarkdown.ts'

const FIRST_ID = '93514f58-ee1d-6874-ac0b-f3ee5edbe574'
const SECOND_ID = '93514f58-ee1d-6874-ac0b-f3ee5edbe575'

test('appends full content for each unique cited chunk', async () => {
  const calls: string[] = []
  const answer = [
    `First claim <kb doc="policy.pdf" chunk_id="${FIRST_ID}" />`,
    `Second claim <kb doc="policy.pdf" chunk_id="${FIRST_ID}" />`,
    `Third claim <kb doc="guide.docx" chunk_id="${SECOND_ID}" />`,
  ].join('\n')

  const result = await buildCitationEnrichedCopyText(answer, [], async (chunkId) => {
    calls.push(chunkId)
    return `Full content for ${chunkId}`
  })

  assert.deepEqual(calls, [FIRST_ID, SECOND_ID])
  assert.equal(result.citationCount, 2)
  assert.deepEqual(result.failedChunkIds, [])
  assert.ok(result.text.startsWith(answer))
  assert.match(result.text, new RegExp(`<kb_chunk_content doc="policy\\.pdf" chunk_id="${FIRST_ID}">`))
  assert.match(result.text, new RegExp(`Full content for ${FIRST_ID}`))
})

test('returns answers without KB citations unchanged', async () => {
  const answer = 'Answer without a knowledge citation.'
  let loaderCalled = false

  const result = await buildCitationEnrichedCopyText(answer, [], async () => {
    loaderCalled = true
    return 'unused'
  })

  assert.equal(result.text, answer)
  assert.equal(result.citationCount, 0)
  assert.equal(loaderCalled, false)
})

test('marks unavailable chunks without discarding the answer', async () => {
  const answer = `<kb doc="missing.pdf" chunk_id="${FIRST_ID}" />`
  const result = await buildCitationEnrichedCopyText(answer, [], async () => {
    throw new Error('not found')
  })

  assert.ok(result.text.startsWith(answer))
  assert.match(result.text, /status="unavailable"/)
  assert.deepEqual(result.failedChunkIds, [FIRST_ID])
})

test('resolves positional citation ids before loading chunk content', async () => {
  const refs: CitationKnowledgeRef[] = [{
    id: FIRST_ID,
    knowledge_title: 'policy.pdf',
  }]
  const answer = '<kb doc="policy.pdf" chunk_id="DOC-1" />'
  let loadedId = ''

  await buildCitationEnrichedCopyText(answer, refs, async (chunkId) => {
    loadedId = chunkId
    return 'Resolved content'
  })

  assert.equal(loadedId, FIRST_ID)
})
