/** Shared citation tag preprocessing for chat markdown (QA + agent). */

/** Self-closing or unclosed `<kb/>` / `<web/>` tags from model output. */
export const KB_WEB_TAG_RE = /<(?:kb|web)\b[^>]*?\s*\/?>/g
const KB_TAG_ATTR_RE = /<kb\b([^>]*?)\s*\/?>/g
const WEB_TAG_ATTR_RE = /<web\b([^>]*?)\s*\/?>/g

const ATTRIBUTE_REGEX = /([\w-]+)\s*=\s*"([^"]*)"/g
const UUID_RE = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i

/**
 * Hide a citation tag while the typewriter has only emitted part of it.
 *
 * Without this guard, Markdown renders the leading `<` as ordinary text until
 * the closing `>` arrives. Only the unfinished tail is removed; a complete tag
 * continues through the normal citation pipeline.
 */
export function stripIncompleteCitationTag(content: string): string {
  if (!content) return content

  const start = content.lastIndexOf('<')
  if (start < 0) return content

  const tail = content.slice(start)
  if (tail.includes('>')) return content

  const isCitationPrefix = tail === '<'
    || /^<k(?:b(?:\s[\s\S]*)?)?$/i.test(tail)
    || /^<w(?:e(?:b(?:\s[\s\S]*)?)?)?$/i.test(tail)

  return isCitationPrefix ? content.slice(0, start) : content
}

export type CitationKnowledgeRef = {
  id?: string
  chunk_ids?: string[]
  knowledge_id?: string
  knowledge_title?: string
  knowledge_filename?: string
  chunk_index?: number
  chunk_type?: string
  knowledge_base_id?: string
}

export type KnowledgeCitationTag = {
  rawTag: string
  doc: string
  rawChunkId: string
  chunkId: string
  kbId: string
}

export type CitationEnrichedCopyResult = {
  text: string
  citationCount: number
  failedChunkIds: string[]
}

export type ResolvedCitationChunk = {
  citation: KnowledgeCitationTag
  content: string
  failed: boolean
}

function parseTagAttributes(attrString: string): Record<string, string> {
  const attributes: Record<string, string> = {}
  if (!attrString) return attributes
  ATTRIBUTE_REGEX.lastIndex = 0
  let match: RegExpExecArray | null
  while ((match = ATTRIBUTE_REGEX.exec(attrString)) !== null) {
    attributes[match[1]] = match[2]
  }
  return attributes
}

function escapeHtml(text: string): string {
  return String(text)
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
}

function truncateMiddle(text: string, maxLength = 13): string {
  if (!text) return ''
  if (text.length <= maxLength) return text
  const half = Math.floor((maxLength - 3) / 2)
  const start = text.slice(0, half + ((maxLength - 3) % 2))
  const end = text.slice(-half)
  return `${start}...${end}`
}

function normalizeDocTitle(title: string): string {
  return title.trim().toLowerCase()
}

function docTitlesMatch(a: string, b: string): boolean {
  if (!a || !b) return false
  const na = normalizeDocTitle(a)
  const nb = normalizeDocTitle(b)
  return na === nb || na.includes(nb) || nb.includes(na)
}

/** Map model context index (1, FAQ-1, DOC-2) to the real chunk UUID from retrieval results. */
export function resolveCitationChunkId(
  rawChunkId: string,
  attrs: { doc?: string; kbId?: string },
  refs?: CitationKnowledgeRef[] | null,
): string {
  const raw = String(rawChunkId || '').trim()
  if (!raw || UUID_RE.test(raw)) return raw

  const list = (refs || []).filter((r) => r)
  if (!list.length) return raw

  const doc = (attrs.doc || '').trim()
  const kbId = (attrs.kbId || '').trim()

  if (doc) {
    const byDoc = list.find(
      (r) =>
        docTitlesMatch(doc, r.knowledge_title || '') ||
        docTitlesMatch(doc, r.knowledge_filename || ''),
    )
    if (byDoc?.id) return byDoc.id
  }

  const faqMatch = raw.match(/^FAQ-(\d+)$/i)
  if (faqMatch) {
    const faqRefs = list.filter((r) => r.chunk_type === 'faq')
    const hit = faqRefs[parseInt(faqMatch[1], 10) - 1]
    if (hit?.id) return hit.id
  }

  const docMatch = raw.match(/^DOC-(\d+)$/i)
  if (docMatch) {
    const docRefs = list.filter((r) => r.chunk_type !== 'faq')
    const hit = docRefs[parseInt(docMatch[1], 10) - 1]
    if (hit?.id) return hit.id
  }

  const num = parseInt(raw, 10)
  if (!Number.isNaN(num) && String(num) === raw) {
    const byPos = list[num - 1]
    if (byPos?.id) return byPos.id
    const byChunkIndex = list.find((r) => r.chunk_index === num || r.chunk_index === num - 1)
    if (byChunkIndex?.id) return byChunkIndex.id
  }

  if (kbId) {
    const scoped = list.filter((r) => r.knowledge_base_id === kbId)
    if (doc) {
      const byDoc = scoped.find(
        (r) =>
          docTitlesMatch(doc, r.knowledge_title || '') ||
          docTitlesMatch(doc, r.knowledge_filename || ''),
      )
      if (byDoc?.id) return byDoc.id
    }
    if (scoped.length === 1 && scoped[0].id) return scoped[0].id
  }

  return raw
}

/** Convert <web/> and <kb/> tags into inline citation HTML. */
export function preprocessCitationTags(
  contentStr: string,
  refs?: CitationKnowledgeRef[] | null,
): string {
  if (!contentStr.trim()) return ''

  return contentStr
    .replace(WEB_TAG_ATTR_RE, (_m, attrString: string) => {
      const attrs = parseTagAttributes(attrString)
      const url = attrs.url || ''
      const title = attrs.title || ''
      if (!url) return ''

      let domain = url
      try {
        const u = new URL(url)
        const host = u.hostname || ''
        const parts = host.split('.')
        domain = parts.length >= 2 ? parts.slice(-2).join('.') : host || url
      } catch {
        // keep original
      }
      const safeTitle = escapeHtml(title)
      const safeUrl = escapeHtml(url)
      return `<a class="citation citation-web" data-url="${safeUrl}" href="${safeUrl}" target="_blank" rel="noopener noreferrer"><span class="citation-icon citation-icon--web" aria-hidden="true"></span><span class="citation-domain">${domain}</span><span class="citation-tip"><span class="tip-title">${safeTitle}</span><span class="tip-url">${safeUrl}</span></span></a>`
    })
    .replace(KB_TAG_ATTR_RE, (_m, attrString: string) => {
      const attrs = parseTagAttributes(attrString)
      const doc = attrs.doc || ''
      const rawChunkId = attrs.chunk_id || attrs.chunkId || ''
      const kbId = attrs.kb_id || attrs.kbId || ''
      const chunkId = resolveCitationChunkId(rawChunkId, { doc, kbId }, refs)
      if (!doc || !chunkId) return ''

      const safeDoc = escapeHtml(doc)
      const safeKbId = escapeHtml(kbId)
      const safeChunkId = escapeHtml(chunkId)
      const displayDoc = escapeHtml(truncateMiddle(doc))
      return `<span class="citation citation-kb" data-kb-id="${safeKbId}" data-chunk-id="${safeChunkId}" data-doc="${safeDoc}" role="button" tabindex="0"><span class="citation-icon citation-icon--book" aria-hidden="true"></span><span class="citation-text">${displayDoc}</span><span class="citation-tip"><span class="tip-loading">…</span></span></span>`
    })
}

/** Extract knowledge citations from model output and resolve context indexes to chunk UUIDs. */
export function extractKnowledgeCitationTags(
  content: string,
  refs?: CitationKnowledgeRef[] | null,
): KnowledgeCitationTag[] {
  if (!content) return []

  const citations: KnowledgeCitationTag[] = []
  const matcher = new RegExp(KB_TAG_ATTR_RE.source, KB_TAG_ATTR_RE.flags)
  let match: RegExpExecArray | null
  while ((match = matcher.exec(content)) !== null) {
    const attrs = parseTagAttributes(match[1] || '')
    const doc = attrs.doc || ''
    const rawChunkId = attrs.chunk_id || attrs.chunkId || ''
    const kbId = attrs.kb_id || attrs.kbId || ''
    const chunkId = resolveCitationChunkId(rawChunkId, { doc, kbId }, refs)
    if (!chunkId) continue

    citations.push({
      rawTag: match[0],
      doc,
      rawChunkId,
      chunkId,
      kbId,
    })
  }

  return citations
}

function escapeCopyAttribute(value: string): string {
  return String(value)
    .replace(/&/g, '&amp;')
    .replace(/"/g, '&quot;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
}

/**
 * Resolve and load every unique knowledge citation in model-output order.
 */
export async function resolveCitationChunks(
  content: string,
  refs: CitationKnowledgeRef[] | null | undefined,
  loadChunkContent: (chunkId: string) => Promise<string>,
): Promise<ResolvedCitationChunk[]> {
  const answer = String(content || '').trim()
  const uniqueCitations = Array.from(
    new Map(
      extractKnowledgeCitationTags(answer, refs).map((citation) => [citation.chunkId, citation]),
    ).values(),
  )

  if (!uniqueCitations.length) return []

  return Promise.all(
    uniqueCitations.map(async (citation) => {
      try {
        const loadedContent = String(await loadChunkContent(citation.chunkId)).trim()
        if (!loadedContent) throw new Error('Chunk content is empty')
        return { citation, content: loadedContent, failed: false }
      } catch {
        return { citation, content: '', failed: true }
      }
    }),
  )
}

/**
 * Preserve the original answer and append the full content for each unique KB citation.
 * The loader is injected so the formatting logic remains independent of the API client.
 */
export async function buildCitationEnrichedCopyText(
  content: string,
  refs: CitationKnowledgeRef[] | null | undefined,
  loadChunkContent: (chunkId: string) => Promise<string>,
): Promise<CitationEnrichedCopyResult> {
  const answer = String(content || '').trim()
  const results = await resolveCitationChunks(answer, refs, loadChunkContent)

  if (!results.length) {
    return { text: answer, citationCount: 0, failedChunkIds: [] }
  }

  const chunkBlocks = results.map(({ citation, content: chunkContent, failed }) => {
    const attrs = [
      `doc="${escapeCopyAttribute(citation.doc)}"`,
      `chunk_id="${escapeCopyAttribute(citation.chunkId)}"`,
    ]
    if (citation.kbId) {
      attrs.push(`kb_id="${escapeCopyAttribute(citation.kbId)}"`)
    }
    if (failed) {
      attrs.push('status="unavailable"')
      return `<kb_chunk_content ${attrs.join(' ')} />`
    }
    return `<kb_chunk_content ${attrs.join(' ')}>\n${chunkContent}\n</kb_chunk_content>`
  })

  return {
    text: `${answer}\n\n<kb_chunk_contents>\n${chunkBlocks.join('\n\n')}\n</kb_chunk_contents>`,
    citationCount: results.length,
    failedChunkIds: results
      .filter((result) => result.failed)
      .map((result) => result.citation.chunkId),
  }
}

/**
 * 复制时剔除答案中的引用标签噪音，只保留可读的纯文本：
 *   - 内联自闭合引用标记：`<kb .../>`、`<web .../>`
 *   - 由 `buildCitationEnrichedCopyText` 追加的 `<kb_chunk_contents>` 参考块
 *     （含自闭合的 `<kb_chunk_content .../>` 与带内容的
 *     `<kb_chunk_content ...>...</kb_chunk_content>` 子块）
 *
 * 只剥标签及紧贴在前面的空白，标签后留 1 个空白占位（避免把
 * `Web ref <web/> here` 拼成 `Web refhere`）。最终再做一遍后处理
 * 清理：折叠多余空格、剔除句末标点前的空格。空字符串原样返回。
 */
export function stripCitationTagsForCopy(text: string): string {
  if (!text) return text
  return text
    // 整块 <kb_chunk_contents>...</kb_chunk_contents> 参考块，连同前面的分隔空白
    .replace(/\s*<kb_chunk_contents\b[^>]*>[\s\S]*?<\/kb_chunk_contents>/gi, '')
    // 自闭合 <kb_chunk_content .../>（失败的 chunk），连同前面的空白
    .replace(/\s*<kb_chunk_content\b[^>]*?\/>/gi, '')
    // 带内容的 <kb_chunk_content ...>...</kb_chunk_content>，连同前面的空白
    .replace(/\s*<kb_chunk_content\b[^>]*>[\s\S]*?<\/kb_chunk_content>/gi, '')
    // 内联自闭合 <kb .../>，连同前面的空白
    .replace(/\s*<kb\b[^>]*?\/>/gi, '')
    // 内联自闭合 <web .../>，连同前面的空白
    .replace(/\s*<web\b[^>]*?\/>/gi, '')
    // 后处理：折叠 2+ 连续空格 / Tab 为单空格（不折叠换行）
    .replace(/[ \t]{2,}/g, ' ')
    // 后处理：剔除句末标点前的多余空格（`First claim .` → `First claim.`）
    .replace(/ ([.,;:!?])/g, '$1')
    // 后处理：剥掉字符串末尾残留的空白
    .replace(/[ \t]+$/g, '')
}

const HTML_PLACEHOLDER_RE = /@@ROCHE_KAP_HTML_PLACEHOLDER_(\d+)@@/g

/** Protect citation HTML from markdown parser; restore after marked.parse. */
export function extractCitationHtmlPlaceholders(
  contentStr: string,
  refs?: CitationKnowledgeRef[] | null,
): { content: string; htmlSnippets: string[] } {
  const htmlSnippets: string[] = []
  const storeHtml = (html: string): string => {
    const idx = htmlSnippets.length
    htmlSnippets.push(html)
    return `@@ROCHE_KAP_HTML_PLACEHOLDER_${idx}@@`
  }

  const content = contentStr
    .replace(KB_WEB_TAG_RE, (match) => storeHtml(preprocessCitationTags(match, refs)))
    .replace(/\[\[([^\]]+)\]\]/g, (match) => storeHtml(preprocessCitationTags(match, refs)))

  return { content, htmlSnippets }
}

export function restoreCitationHtmlPlaceholders(html: string, htmlSnippets: string[]): string {
  if (!htmlSnippets.length) return html
  return html.replace(HTML_PLACEHOLDER_RE, (_match, idx) => htmlSnippets[Number(idx)] || '')
}

/** Opening/closing fence for GFM fenced code blocks (up to 3 spaces indent). */
const FENCED_CODE_DELIMITER_RE = /^ {0,3}(`{3,}|~{3,})(\s*\S.*)?\s*$/

function isFencedCodeDelimiterLine(line: string): boolean {
  return FENCED_CODE_DELIMITER_RE.test(line)
}

/** Collapse newlines around <kb/> / <web/> so marked keeps citations inline. */
export function joinCitationTagsToPreviousLine(content: string): string {
  if (!content) return content

  let result = content

  // Newlines between consecutive citation tags
  let prev = ''
  while (result !== prev) {
    prev = result
    result = result.replace(
      /(<(?:kb|web)\b[^>]*?\s*\/?>)\s*\n+\s*(<(?:kb|web)\b)/gi,
      '$1 $2',
    )
  }

  // Blank lines before citations: join to the previous content. Fenced-code
  // delimiters are the only exception because ``` / ~~~ must stay on their own line.
  result = result.replace(/\n[ \t]*\n+([ \t]*<(?:kb|web)\b)/gi, (match, kbStart, offset, full) => {
    const before = full.slice(0, offset)
    const lastLine = before.split('\n').filter((line: string) => line.trim()).pop() || ''
    if (isFencedCodeDelimiterLine(lastLine)) {
      return `\n\n${kbStart}`
    }
    return ` ${kbStart.trimStart()}`
  })

  // Single newline before citation when it follows text or another citation (not after a blank line)
  result = result.replace(
    /(?<!\n)(<(?:kb|web)\b[^>]*?\s*\/?>|[ \t]*\S[^\n]*?)\n([ \t]*<(?:kb|web)\b)/g,
    (match, beforePart: string, kbStart: string, offset: number, full: string) => {
      // Resolve the full preceding line: lazy capture + lookbehind can grab only a
      // partial line (e.g. ``` captured as ``), which would skip the fence check.
      const lineStart = full.lastIndexOf('\n', offset - 1) + 1
      const fullPrevLine = full.slice(lineStart, offset + beforePart.length)
      if (isFencedCodeDelimiterLine(fullPrevLine)) {
        return match
      }
      return `${beforePart} ${kbStart.trimStart()}`
    },
  )

  return result
}

const CITATION_HTML_FRAGMENT =
  '(?:<span class="citation\\b[^]*?</span>|<a class="citation\\b[^]*?</a>)'

/** Merge citation-only <p> blocks into the preceding paragraph (marked splits on newlines). */
export function collapseStandaloneCitationParagraphs(html: string): string {
  if (!html || !html.includes('citation')) return html

  const mergePattern = new RegExp(
    `(<\\/(?:p|li)>)\\s*(?:<p>\\s*<\\/p>\\s*)*<p>\\s*(${CITATION_HTML_FRAGMENT})\\s*<\\/p>`,
    'g',
  )

  let result = html
  let prev = ''
  while (result !== prev) {
    prev = result
    result = result.replace(mergePattern, (_match, closeTag: string, citation: string) => {
      return ` ${citation}${closeTag}`
    })
  }

  return result
}

/** Preserve raw <kb>/<web> tags before sanitizers that would strip them. */
export function preserveCitationTags(contentStr: string): { text: string; tags: string[] } {
  const tags: string[] = []
  const text = contentStr.replace(KB_WEB_TAG_RE, (match) => {
    const idx = tags.length
    tags.push(match)
    return `\x00TAG${idx}\x00`
  })
  return { text, tags }
}

export function restoreCitationTags(text: string, tags: string[]): string {
  return text.replace(/\x00TAG(\d+)\x00/g, (_, idx) => tags[Number(idx)] || '')
}
