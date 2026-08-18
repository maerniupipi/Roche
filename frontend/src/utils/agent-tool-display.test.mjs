import assert from 'node:assert/strict'
import test from 'node:test'
import { getAgentToolIconName } from './agent-tool-icons.ts'
import {
  getKnowledgeSearchSummaryHtml,
  getQueryText,
  getRagPipelineStepTitle,
} from './agent-tool-display.ts'

const t = (key, params) => {
  if (key === 'agentStream.ragPipeline.searchingWithQuery') {
    return `searching ${params?.query}`
  }
  return key
}

test('getAgentToolIconName maps rag pipeline tools', () => {
  assert.equal(getAgentToolIconName('query_understand'), 'ai-search')
  assert.equal(getAgentToolIconName('knowledge_search'), 'data-search')
})

test('getQueryText joins unique query strings', () => {
  assert.equal(getQueryText({ query: 'foo', queries: ['foo', 'bar'] }), 'foo，bar')
})

test('getQueryText parses JSON-encoded queries string', () => {
  assert.equal(
    getQueryText({
      queries: '["合力天胜游泳俱乐部介绍", "合力天胜游泳训练机构", "合力天胜游泳队"]',
    }),
    '合力天胜游泳俱乐部介绍，合力天胜游泳训练机构，合力天胜游泳队',
  )
})

test('getKnowledgeSearchSummaryHtml returns foundResults regardless of kb_counts', () => {
  // 历史合成的 tool_data 即使带 kb_counts，折叠摘要也走 foundResults 与流式保持一致。
  const html = getKnowledgeSearchSummaryHtml(t, {
    results: [{}, {}],
    kb_counts: { a: 1, b: 2 },
  })
  assert.equal(html, 'agentStream.search.foundResults')
})

test('getKnowledgeSearchSummaryHtml prefers toolData.count over results.length', () => {
  // 历史合成的 tool_data：results.length=1（按文档去重后的引用数），
  // count=30（progress_result_count，即流式 chunk 总数），
  // 折叠摘要应展示 30，与流式端保持一致。
  let capturedKey = ''
  let capturedParams = null
  const captureT = (key, params) => {
    capturedKey = key
    capturedParams = params
    return key
  }
  const html = getKnowledgeSearchSummaryHtml(captureT, {
    results: [{ knowledge_id: 'k1' }],
    count: 30,
    kb_counts: { k1: 1 },
  })
  assert.equal(html, 'agentStream.search.foundResults')
  assert.equal(capturedKey, 'agentStream.search.foundResults')
  assert.equal(capturedParams?.count, '<strong>30</strong>')
})

test('getKnowledgeSearchSummaryHtml falls back to results.length when count is missing', () => {
  // 流式场景：tool_data 只有 results，没有 count，应该用 results.length。
  let capturedKey = ''
  let capturedParams = null
  const captureT = (key, params) => {
    capturedKey = key
    capturedParams = params
    return key
  }
  const html = getKnowledgeSearchSummaryHtml(captureT, {
    results: [{ knowledge_id: 'k1' }, { knowledge_id: 'k2' }, { knowledge_id: 'k3' }],
  })
  assert.equal(html, 'agentStream.search.foundResults')
  assert.equal(capturedKey, 'agentStream.search.foundResults')
  assert.equal(capturedParams?.count, '<strong>3</strong>')
})

test('getRagPipelineStepTitle uses query-aware search labels', () => {
  const title = getRagPipelineStepTitle(t, {
    tool_name: 'knowledge_search',
    pending: true,
    arguments: { query: '讯飞开放平台' },
  })
  assert.equal(title, 'searching 讯飞开放平台')
})

test('getRagPipelineStepTitle falls back to key for unknown search_source', () => {
  const title = getRagPipelineStepTitle(t, {
    tool_name: 'knowledge_search',
    pending: false,
    success: true,
    arguments: { search_source: 'unknown' },
  })
  assert.equal(title, 'agentStream.toolStatus.searchKb')
})
