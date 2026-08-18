import { get } from "../../utils/request";

export interface KnowledgeBaseStats {
  published_count: number;
  upload_success_count: number;
  upload_failed_count: number;
  scheduled_publish_count: number;
  unpublished_count: number;
  archived_count: number;
}

export interface DailyChatStat {
  date: string;
  question_count: number;
  unique_users: number;
  satisfaction_pct: number;
}

export interface ChatStats {
  avg_first_response_sec: number;
  avg_complete_sec: number;
  daily: DailyChatStat[];
}

export interface DomainDistributionItem {
  name: string;
  value: number;
}

export interface TopDocumentItem {
  rank: number;
  title: string;
  kb_name: string;
  hit_count: number;
}

export interface FeedbackItem {
  category: string;
  count: number;
}

export interface TopUserItem {
  rank: number;
  user_name: string;
  question_count: number;
}

export interface FallbackQuestionItem {
  rank: number;
  content: string;
}

export interface OverviewData {
  domain_distribution: DomainDistributionItem[];
  cross_domain_single: number;
  cross_domain_multi: number;
  top_documents: TopDocumentItem[];
  product_feedback: FeedbackItem[];
  top_users: TopUserItem[];
  valid_answer_count: number;
  fallback_answer_count: number;
  fallback_questions: FallbackQuestionItem[];
}

export function getKnowledgeBaseStats(knowledgeBaseId?: string) {
  const qs = new URLSearchParams();
  if (knowledgeBaseId) qs.set("knowledge_base_id", knowledgeBaseId);
  return get<{ success: boolean; data: KnowledgeBaseStats }>(
    `/api/v1/dashboard/knowledge-base-stats?${qs.toString()}`
  );
}

export function getChatStats(params: {
  knowledge_domain_id?: number;
  start_date: string;
  end_date: string;
}) {
  const qs = new URLSearchParams();
  if (params.knowledge_domain_id !== undefined) {
    qs.set("knowledge_domain_id", String(params.knowledge_domain_id));
  }
  qs.set("start_date", params.start_date);
  qs.set("end_date", params.end_date);
  return get<{ success: boolean; data: ChatStats }>(
    `/api/v1/dashboard/chat-stats?${qs.toString()}`
  );
}

export function getOverview(params: {
  knowledge_domain_id?: number;
  start_date: string;
  end_date: string;
}) {
  const qs = new URLSearchParams();
  if (params.knowledge_domain_id !== undefined) {
    qs.set("knowledge_domain_id", String(params.knowledge_domain_id));
  }
  qs.set("start_date", params.start_date);
  qs.set("end_date", params.end_date);
  return get<{ success: boolean; data: OverviewData }>(
    `/api/v1/dashboard/overview?${qs.toString()}`
  );
}
