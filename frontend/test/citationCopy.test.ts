import assert from 'node:assert/strict'
import test from 'node:test'

import {
  buildCitationEnrichedCopyText,
  stripCitationTagsForCopy,
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

test('stripCitationTagsForCopy removes inline <kb/> and <web/> self-closing tags', () => {
  const input = [
    `First claim <kb doc="policy.pdf" chunk_id="${FIRST_ID}" />.`,
    `Web ref <web url="https://example.com" title="Example" /> here.`,
  ].join('\n')

  const result = stripCitationTagsForCopy(input)
  assert.equal(
    result,
    'First claim.\nWeb ref here.',
  )
  assert.doesNotMatch(result, /<kb\b/i)
  assert.doesNotMatch(result, /<web\b/i)
})

test('stripCitationTagsForCopy removes <kb_chunk_contents> wrapper and inner <kb_chunk_content> blocks', () => {
  const input = [
    `Answer body stays here.`,
    ``,
    `<kb_chunk_contents>`,
    `<kb_chunk_content doc="policy.pdf" chunk_id="${FIRST_ID}">`,
    `Full content for chunk A`,
    `</kb_chunk_content>`,
    ``,
    `<kb_chunk_content doc="guide.docx" chunk_id="${SECOND_ID}" status="unavailable" />`,
    `</kb_chunk_contents>`,
  ].join('\n')

  const result = stripCitationTagsForCopy(input)
  assert.equal(result, 'Answer body stays here.')
  assert.doesNotMatch(result, /<kb_chunk_contents\b/i)
  assert.doesNotMatch(result, /<kb_chunk_content\b/i)
})

test('stripCitationTagsForCopy returns text unchanged when no tags are present', () => {
  const input = 'Plain answer with no citation tags at all.'
  assert.equal(stripCitationTagsForCopy(input), input)
})

test('stripCitationTagsForCopy handles empty / falsy input gracefully', () => {
  assert.equal(stripCitationTagsForCopy(''), '')
  // @ts-expect-error verifying runtime guard for accidental non-string input
  assert.equal(stripCitationTagsForCopy(undefined), undefined)
})

test('copy path strips tags end-to-end via buildCitationEnrichedCopyText', async () => {
  const answer = [
    `Answer <kb doc="policy.pdf" chunk_id="${FIRST_ID}" />.`,
    `Reference <web url="https://example.com" title="Example" />.`,
  ].join('\n')

  const enriched = await buildCitationEnrichedCopyText(answer, [], async () => 'Resolved body')
  const cleaned = stripCitationTagsForCopy(enriched.text)

  assert.equal(cleaned, 'Answer.\nReference.')
  assert.doesNotMatch(cleaned, /<kb\b/i)
  assert.doesNotMatch(cleaned, /<web\b/i)
  assert.doesNotMatch(cleaned, /<kb_chunk_contents\b/i)
  assert.doesNotMatch(cleaned, /<kb_chunk_content\b/i)
})
