/**
 * Tool Capability Requirements
 *
 * Single source of truth for the mapping from agent tool names to the
 * knowledge-base capabilities each tool depends on. Used to:
 *   - Gray out tools whose dependencies aren't satisfied by the current scope
 *     (see `AgentEditorModal.vue` → `availableTools`).
 *
 * Capability names mirror the backend's KB capability set:
 *   - vector:  vector chunk index (embedding-based retrieval)
 *   - keyword: keyword/BM25 chunk index
 *   - graph:   knowledge graph index
 *   - faq:     FAQ-type KB (question/answer pairs as chunks)
 *
 * Requirement semantics:
 *   - `anyOf`: KB scope must expose at least ONE listed capability.
 *   - `allOf`: KB scope must expose ALL listed capabilities.
 *   - empty  : tool has no KB requirement (always available).
 *
 * IMPORTANT: keep this list aligned with backend tool definitions
 * (`internal/agent/tools/`). A tool missing from this map defaults to
 * "always available" so new tools don't silently start disabled.
 */

export type KBCapability = 'vector' | 'keyword' | 'graph' | 'faq';

export interface ToolRequirement {
  anyOf?: KBCapability[];
  allOf?: KBCapability[];
  /**
   * Whether this tool can use user-provided file references (via @ 提及) as
   * an additional retrieval scope. Tools with `consumesFiles: false` ignore
   * `knowledge_ids`; we use this flag in the chat `@` dropdown to decide
   * whether to even offer the "文件" tab to the user.
   */
  consumesFiles?: boolean;
}

export const TOOL_CAPABILITY_REQUIREMENTS: Record<string, ToolRequirement> = {
  // ---- base / reasoning (no KB dependency) ----
  thinking: {},
  todo_write: {},

  // ---- RAG / chunk retrieval (need at least one chunk-indexed KB) ----
  // We use vector|keyword as the canonical "has RAG chunks" signal. FAQ KBs
  // also expose chunks, but the current UX message bucket is "RAG KB"; once
  // we add a dedicated `requiresFaqKb` i18n key we can include `faq` here.
  knowledge_search:      { anyOf: ['vector', 'keyword'], consumesFiles: true },
  grep_chunks:           { anyOf: ['vector', 'keyword'], consumesFiles: true },
  list_knowledge_chunks: { anyOf: ['vector', 'keyword'], consumesFiles: true },
  query_knowledge_graph: { anyOf: ['vector', 'keyword'], consumesFiles: true },
  get_document_info:     { anyOf: ['vector', 'keyword'], consumesFiles: true },
  database_query:        { anyOf: ['vector', 'keyword'], consumesFiles: true },

  // ---- Data analysis (reads table summary/column chunks produced by RAG ingest) ----
  data_analysis: { anyOf: ['vector', 'keyword'], consumesFiles: true },
  data_schema:   { anyOf: ['vector', 'keyword'], consumesFiles: true },
};

/**
 * Aggregate KB-capability set available in the current user's authorized KB scope.
 * All fields default to `false` for unknown capabilities.
 */
export interface ScopeCapabilities {
  vector: boolean;
  keyword: boolean;
  graph: boolean;
  faq: boolean;
}

/**
 * Machine-readable reason a tool is unsatisfiable. Map to a user-facing
 * string via i18n on the caller side (see `AgentEditorModal.vue`).
 */
export type RequirementMissKind = 'none' | 'needsKb' | 'needsRag' | 'needsGraph' | 'needsFaq';

/**
 * Evaluate whether a tool's requirements are satisfied by the scope.
 *
 * @param toolName  the tool identifier (see `TOOL_CAPABILITY_REQUIREMENTS`)
 * @param scope     aggregate capabilities exposed by KBs currently in scope
 * @param hasAnyKb  whether the current user has at least one accessible KB
 */
export function evaluateToolRequirement(
  toolName: string,
  scope: ScopeCapabilities,
  hasAnyKb: boolean,
): { ok: boolean; missKind: RequirementMissKind } {
  const req = TOOL_CAPABILITY_REQUIREMENTS[toolName];
  // Tools absent from the map or with no requirements: always available.
  if (!req || (!req.anyOf?.length && !req.allOf?.length)) {
    return { ok: true, missKind: 'none' };
  }

  // Any capability requirement implies needing at least one KB in scope.
  if (!hasAnyKb) return { ok: false, missKind: 'needsKb' };

  const has = (c: KBCapability): boolean => !!scope[c];

  if (req.allOf && req.allOf.length > 0) {
    for (const c of req.allOf) {
      if (!has(c)) return { ok: false, missKind: primaryMissKind(c) };
    }
  }
  if (req.anyOf && req.anyOf.length > 0) {
    if (!req.anyOf.some(has)) {
      return { ok: false, missKind: primaryMissKind(req.anyOf[0]) };
    }
  }
  return { ok: true, missKind: 'none' };
}

function primaryMissKind(c: KBCapability): RequirementMissKind {
  switch (c) {
    case 'graph':   return 'needsGraph';
    case 'faq':     return 'needsFaq';
    case 'vector':
    case 'keyword': return 'needsRag';
  }
}

/**
 * True iff any of the agent's allowed tools can consume user-provided file
 * references (`knowledge_ids`). Used by the chat `@` menu to decide whether
 * showing the "文件" list makes sense at all. `undefined`/`null`/empty → true
 * (permissive fallback: if we don't
 * know, show files).
 */
export function toolsConsumeFiles(
  allowedTools: string[] | undefined | null,
): boolean {
  if (!allowedTools || allowedTools.length === 0) return true;
  for (const t of allowedTools) {
    const req = TOOL_CAPABILITY_REQUIREMENTS[t];
    // Unknown tools are treated as potentially file-consuming.
    if (!req) return true;
    if (req.consumesFiles) return true;
  }
  return false;
}
