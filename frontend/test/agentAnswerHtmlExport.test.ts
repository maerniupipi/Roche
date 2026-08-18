import assert from 'node:assert/strict'
import test from 'node:test'

import {
  buildAgentAnswerExportFileName,
  buildAgentAnswerExportHtml,
} from '../src/utils/agentAnswerHtmlExport.ts'
import type { ResolvedCitationChunk } from '../src/utils/citationMarkdown.ts'

const CHUNK_ID = '93514f58-ee1d-6874-ac0b-f3ee5edbe574'

const chunks: ResolvedCitationChunk[] = [{
  citation: {
    rawTag: `<kb doc="policy.pdf" chunk_id="${CHUNK_ID}" />`,
    doc: 'policy.pdf',
    rawChunkId: CHUNK_ID,
    chunkId: CHUNK_ID,
    kbId: '',
  },
  content: 'Full policy chunk',
  failed: false,
}]

test('uses the user question as a safe HTML file name', () => {
  assert.equal(
    buildAgentAnswerExportFileName('Who approves travel: China / APAC?'),
    'Who approves travel China APAC.html',
  )
  assert.equal(buildAgentAnswerExportFileName('  '), 'agent-answer.html')
})

test('builds a standalone answer page with interactive chunk references', () => {
  const html = buildAgentAnswerExportHtml({
    title: 'Who approves travel?',
    answerHtml: `<p>Answer <span class="citation-kb" data-chunk-id="${CHUNK_ID}">policy.pdf</span></p>`,
    chunks,
    labels: {
      references: 'References',
      close: 'Close',
      unavailable: 'Unavailable',
    },
    locale: 'en-US',
  })

  assert.match(html, /<!doctype html>/)
  assert.match(html, /<article class="answer">/)
  assert.match(html, /<section class="references"/)
  assert.match(html, new RegExp(`data-chunk-id="${CHUNK_ID}"`))
  assert.match(html, /Full policy chunk/)
  assert.match(html, /dialog\.showModal/)
})

test('escapes script-closing text inside chunk JSON', () => {
  const html = buildAgentAnswerExportHtml({
    title: 'Safe export',
    answerHtml: '<p>Safe answer</p>',
    chunks: [{ ...chunks[0], content: '</script><script>alert(1)</script>' }],
    labels: { references: 'References', close: 'Close', unavailable: 'Unavailable' },
  })

  assert.doesNotMatch(html, /const chunkData = .*<\/script><script>/)
  assert.match(html, /\\u003c\/script\\u003e/)
})
