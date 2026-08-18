/**
 * Tool Results Type Definitions
 * TypeScript interfaces for all tool result types
 */

// Relevance levels — values match the backend API response.
// Display labels are resolved via i18n in SearchResults.vue and GraphQueryResults.vue.
export type RelevanceLevel = 'High Relevance' | 'Medium Relevance' | 'Low Relevance' | 'Weak Relevance';

// Display types
export type DisplayType =
    | 'search_results'
    | 'chunk_detail'
    | 'related_chunks'
    | 'knowledge_base_list'
    | 'document_info'
    | 'graph_query_results'
    | 'thinking'
    | 'plan'
    | 'database_query'
    | 'grep_results'
    | 'knowledge_chunks_list';

// Search result item
export interface SearchResultItem {
    result_index: number;
    chunk_id: string;
    content: string;
    score: number;
    relevance_level: RelevanceLevel;
    knowledge_id: string;
    knowledge_base_id?: string;
    knowledge_title: string;
    match_type: string;
    knowledge_base_type?: string;
    // FAQ entries share the owning document's title; the standard question
    // gives each entry a distinct, human-readable label.
    faq_standard_question?: string;
    faq_similar_questions?: string[];
    faq_answers?: string[];
}

// Chunk item
export interface ChunkItem {
    index: number;
    chunk_id: string;
    chunk_index: number;
    content: string;
    knowledge_id: string;
}

// Knowledge base item
export interface KnowledgeBaseItem {
    index: number;
    id: string;
    name: string;
    description: string;
}

// Graph config
export interface GraphConfig {
    nodes: string[];
    relations: string[];
}

// Search results data
export interface SearchResultsData {
    display_type: 'search_results';
    results?: SearchResultItem[];
    count?: number;
    kb_counts?: Record<string, number>;
    query?: string;
    knowledge_base_id?: string;
}

// Chunk detail data
export interface ChunkDetailData {
    display_type: 'chunk_detail';
    chunk_id: string;
    content: string;
    chunk_index: number;
    knowledge_id: string;
    content_length?: number;
}

// Related chunks data
export interface RelatedChunksData {
    display_type: 'related_chunks';
    chunk_id: string;
    relation_type: string;
    count: number;
    chunks: ChunkItem[];
}

// Knowledge base list data
export interface KnowledgeBaseListData {
    display_type: 'knowledge_base_list';
    knowledge_bases: KnowledgeBaseItem[];
    count: number;
}

// Document info data
export interface DocumentInfoDocument {
    knowledge_id?: string;
    faq_id?: string;
    chunk_id?: string;
    title: string;
    faq_question?: string;
    faq_answers?: string[];
    faq_similar_questions?: string[];
    is_faq?: boolean;
    description?: string;
    type?: string;
    source?: string;
    channel?: string;
    file_name?: string;
    file_type?: string;
    file_size?: number;
    parse_status?: string;
    chunk_count?: number;
    metadata?: Record<string, any>;
    type_icon?: string;
}

export interface DocumentInfoData {
    display_type: 'document_info';
    documents?: DocumentInfoDocument[];
    total_docs: number;
    requested: number;
    errors?: string[];
    title?: string;
}

// Graph query results data
export interface GraphQueryResultsData {
    display_type: 'graph_query_results';
    results: SearchResultItem[];
    count: number;
    graph_config: GraphConfig;
}

// Thinking data
export interface ThinkingData {
    display_type: 'thinking';
    thought: string;
}

// Plan step
export interface PlanStep {
    id: string;
    description: string;
    tools_to_use?: string[]; // Changed from string to array
    status: 'pending' | 'in_progress' | 'completed' | 'skipped';
}

// Plan data
export interface PlanData {
    display_type: 'plan';
    task: string;
    steps: PlanStep[];
    total_steps: number;
}

// Database query data
export interface DatabaseQueryData {
    display_type: 'database_query';
    columns: string[];
    rows: Array<Record<string, any>>;
    row_count: number;
}

// Grep knowledge aggregation item (legacy, grouped by knowledge_id)
export interface GrepKnowledgeResult {
    knowledge_id: string;
    knowledge_base_id: string;
    knowledge_title: string;
    faq_question?: string;
    title_match?: boolean;
    chunk_hit_count: number;
    match_snippet?: string;
    pattern_counts: Record<string, number>;
    total_pattern_hits: number;
    distinct_patterns: number;
}

// Per-chunk grep hit (preferred for UI — one row per FAQ entry or chunk)
export interface GrepChunkResult {
    chunk_id: string;
    faq_id?: string;
    knowledge_id: string;
    knowledge_base_id: string;
    knowledge_title: string;
    chunk_type?: string;
    index?: number;
    chunk_index?: number;
    faq_question?: string;
    title_match?: boolean;
    match_snippet?: string;
    score?: number;
}

// Grep results data
export interface GrepResultsData {
    display_type: 'grep_results';
    query?: string;
    patterns: string[];
    chunk_results?: GrepChunkResult[];
    knowledge_results: GrepKnowledgeResult[];
    result_count: number;
    document_count?: number;
    total_matches: number;
    knowledge_base_ids?: string[];
    limit?: number;
    max_results: number;
}

// Knowledge chunks list data (list_knowledge_chunks tool)
export interface KnowledgeChunksListData {
    display_type: 'knowledge_chunks_list';
    knowledge_id?: string;
    knowledge_title?: string;
    total_chunks?: number;
    fetched_chunks?: number;
    page?: number;
    page_size?: number;
    faq_question?: string;
    faq_id?: string;
    single_chunk?: boolean;
}

// Union type for all tool result data
export type ToolResultData =
    | SearchResultsData
    | ChunkDetailData
    | RelatedChunksData
    | KnowledgeBaseListData
    | DocumentInfoData
    | GraphQueryResultsData
    | ThinkingData
    | PlanData
    | DatabaseQueryData
    | GrepResultsData
    | KnowledgeChunksListData;

// Action data (from index.vue)
export interface ActionData {
    description: string;
    success: boolean;
    tool_name?: string;
    arguments?: any;
    output?: string;
    error?: string;
    details?: boolean;
    display_type?: DisplayType;
    tool_data?: Record<string, any>;
}
