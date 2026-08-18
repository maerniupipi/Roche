import type { ResolvedCitationChunk } from './citationMarkdown.ts'

export type AgentAnswerExportLabels = {
  references: string
  close: string
  unavailable: string
}

export type AgentAnswerHtmlExportOptions = {
  title: string
  answerHtml: string
  chunks: ResolvedCitationChunk[]
  labels: AgentAnswerExportLabels
  locale?: string
}

function escapeHtml(value: string): string {
  return String(value)
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#39;')
}

function serializeForInlineScript(value: unknown): string {
  return JSON.stringify(value)
    .replace(/&/g, '\\u0026')
    .replace(/</g, '\\u003c')
    .replace(/>/g, '\\u003e')
    .replace(/\u2028/g, '\\u2028')
    .replace(/\u2029/g, '\\u2029')
}

export function buildAgentAnswerExportFileName(question: string): string {
  const normalized = String(question || '')
    .replace(/[\u0000-\u001f<>:"/\\|?*]+/g, ' ')
    .replace(/\s+/g, ' ')
    .trim()
    .replace(/[. ]+$/g, '')
  let base = normalized || 'agent-answer'
  if (/^(con|prn|aux|nul|com[1-9]|lpt[1-9])$/i.test(base)) {
    base = `_${base}`
  }
  return `${Array.from(base).slice(0, 120).join('')}.html`
}

export function buildAgentAnswerExportHtml(options: AgentAnswerHtmlExportOptions): string {
  const { title, answerHtml, chunks, labels } = options
  const locale = options.locale || 'zh-CN'
  const chunkData = Object.fromEntries(
    chunks.map(({ citation, content, failed }) => [
      citation.chunkId,
      { doc: citation.doc, content, failed },
    ]),
  )
  const referenceItems = chunks.map(({ citation, failed }) => `
      <button class="export-reference${failed ? ' is-unavailable' : ''}" type="button"
        data-chunk-id="${escapeHtml(citation.chunkId)}" data-doc="${escapeHtml(citation.doc)}">
        <span class="reference-mark" aria-hidden="true"></span>
        <span class="reference-label">${escapeHtml(citation.doc || citation.chunkId)}</span>
        <code>${escapeHtml(citation.chunkId)}</code>
      </button>`).join('')
  const referencesSection = chunks.length
    ? `<section class="references" aria-labelledby="references-title">
        <h2 id="references-title">${escapeHtml(labels.references)}</h2>
        <div class="reference-list">${referenceItems}
        </div>
      </section>`
    : ''

  return `<!doctype html>
<html lang="${escapeHtml(locale)}">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>${escapeHtml(title)}</title>
  <style>
    :root { color-scheme: light; font-family: Inter, "Segoe UI", "PingFang SC", "Microsoft YaHei", sans-serif; color: #1f2329; background: #fff; }
    * { box-sizing: border-box; }
    body { margin: 0; background: #fff; font-size: 16px; line-height: 1.75; }
    .page { width: min(960px, calc(100% - 40px)); margin: 0 auto; padding: 56px 0 80px; }
    .answer { overflow-wrap: anywhere; }
    .answer > :first-child { margin-top: 0; }
    .answer h1, .answer h2, .answer h3, .answer h4 { color: #16181d; line-height: 1.35; margin: 1.5em 0 .65em; letter-spacing: 0; }
    .answer h1 { font-size: 28px; }
    .answer h2 { font-size: 24px; border-top: 1px solid #e7e9ed; padding-top: 28px; }
    .answer h3 { font-size: 20px; }
    .answer p { margin: .75em 0; }
    .answer ul, .answer ol { padding-left: 1.6em; }
    .answer blockquote { margin: 20px 0; padding: 10px 18px; border-left: 3px solid #d7dbe0; color: #5b616b; background: #fafbfc; }
    .answer pre { overflow: auto; padding: 16px; border-radius: 6px; background: #f5f6f7; white-space: pre-wrap; }
    .answer code { font-family: "Cascadia Code", Consolas, monospace; }
    .chat-markdown-table { overflow-x: auto; margin: 18px 0; border: 1px solid #e2e5e9; border-radius: 8px; }
    .answer table { width: 100%; border-collapse: collapse; line-height: 1.55; }
    .answer th, .answer td { padding: 12px 16px; border-right: 1px solid #e2e5e9; border-bottom: 1px solid #e2e5e9; text-align: left; vertical-align: top; }
    .answer th { background: #f5f6f7; font-weight: 600; }
    .answer tr:last-child td { border-bottom: 0; }
    .answer th:last-child, .answer td:last-child { border-right: 0; }
    .citation-tip { display: none !important; }
    .citation-kb, .export-reference { cursor: pointer; }
    .citation-kb { display: inline-flex; align-items: center; max-width: 240px; margin: 0 3px; padding: 2px 8px; border: 1px solid #d8dce1; border-radius: 6px; color: #59616c; background: #f7f8f9; font-size: 12px; line-height: 20px; vertical-align: baseline; }
    .citation-kb:hover, .citation-kb:focus-visible { color: #00a870; border-color: #00a870; background: #f0fbf7; outline: none; }
    .citation-icon--book, .reference-mark { flex: 0 0 auto; width: 10px; height: 10px; margin-right: 5px; border: 1.5px solid currentColor; border-radius: 2px; }
    .citation-text { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
    .citation-web { color: #007a55; }
    .references { margin-top: 48px; padding-top: 28px; border-top: 1px solid #e7e9ed; }
    .references h2 { margin: 0 0 16px; font-size: 20px; }
    .reference-list { display: grid; gap: 10px; }
    .export-reference { width: 100%; display: grid; grid-template-columns: auto minmax(0, 1fr) auto; align-items: center; gap: 8px; padding: 11px 13px; border: 1px solid #dfe3e8; border-radius: 6px; color: #30343b; background: #fff; text-align: left; font: inherit; }
    .export-reference:hover, .export-reference:focus-visible { border-color: #00a870; background: #f5fcf9; outline: none; }
    .export-reference .reference-label { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
    .export-reference code { color: #7a818b; font-size: 12px; }
    .export-reference.is-unavailable { opacity: .65; }
    dialog { width: min(760px, calc(100% - 32px)); max-height: min(78vh, 760px); padding: 0; border: 0; border-radius: 8px; box-shadow: 0 18px 55px rgba(0, 0, 0, .2); color: #20242a; }
    dialog::backdrop { background: rgba(15, 20, 26, .42); }
    .dialog-header { position: sticky; top: 0; display: flex; align-items: flex-start; gap: 18px; padding: 18px 20px; border-bottom: 1px solid #e8eaed; background: #fff; }
    .dialog-heading { min-width: 0; flex: 1; }
    .dialog-heading strong { display: block; color: #00a870; overflow-wrap: anywhere; }
    .dialog-heading code { display: block; margin-top: 4px; color: #848b95; font-size: 12px; overflow-wrap: anywhere; }
    .dialog-close { flex: 0 0 auto; width: 32px; height: 32px; padding: 0; border: 0; border-radius: 5px; background: #f1f3f5; color: #5f6670; font-size: 22px; cursor: pointer; }
    .dialog-content { margin: 0; padding: 20px; max-height: calc(78vh - 82px); overflow: auto; white-space: pre-wrap; overflow-wrap: anywhere; font: 14px/1.7 "Cascadia Code", Consolas, "Microsoft YaHei", monospace; }
    @media (max-width: 640px) { .page { width: min(100% - 24px, 960px); padding-top: 28px; } .export-reference { grid-template-columns: auto minmax(0, 1fr); } .export-reference code { grid-column: 2; } }
  </style>
</head>
<body>
  <main class="page">
    <article class="answer">${answerHtml}</article>
    ${referencesSection}
  </main>
  <dialog id="chunk-dialog">
    <div class="dialog-header">
      <div class="dialog-heading"><strong id="chunk-title"></strong><code id="chunk-id"></code></div>
      <button class="dialog-close" id="dialog-close" type="button" aria-label="${escapeHtml(labels.close)}">&times;</button>
    </div>
    <pre class="dialog-content" id="chunk-content"></pre>
  </dialog>
  <script>
    const chunkData = ${serializeForInlineScript(chunkData)};
    const unavailableLabel = ${serializeForInlineScript(labels.unavailable)};
    const dialog = document.getElementById('chunk-dialog');
    const title = document.getElementById('chunk-title');
    const id = document.getElementById('chunk-id');
    const content = document.getElementById('chunk-content');
    document.addEventListener('click', (event) => {
      const trigger = event.target instanceof Element ? event.target.closest('[data-chunk-id]') : null;
      if (!trigger) return;
      const chunkId = trigger.getAttribute('data-chunk-id') || '';
      const chunk = chunkData[chunkId];
      if (!chunk) return;
      event.preventDefault();
      title.textContent = chunk.doc || trigger.getAttribute('data-doc') || chunkId;
      id.textContent = chunkId;
      content.textContent = chunk.failed ? unavailableLabel : chunk.content;
      if (typeof dialog.showModal === 'function') dialog.showModal();
      else dialog.setAttribute('open', '');
    });
    document.getElementById('dialog-close').addEventListener('click', () => dialog.close());
    dialog.addEventListener('click', (event) => { if (event.target === dialog) dialog.close(); });
  </script>
</body>
</html>`
}
