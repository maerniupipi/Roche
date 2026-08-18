import { get, post, put, del } from "../../utils/request";



export async function createSessions(data = {}) {
  return post("/api/v1/sessions", data);
}

export async function getSessionsList(page: number, page_size: number) {
  const params = new URLSearchParams({ page: String(page), page_size: String(page_size) });
  return get(`/api/v1/sessions?${params.toString()}`);
}

export async function pinSession(session_id: string) {
  return post(`/api/v1/sessions/${session_id}/pin`, {});
}

export async function unpinSession(session_id: string) {
  return del(`/api/v1/sessions/${session_id}/pin`);
}

export async function generateSessionsTitle(session_id: string, data: any) {
  return post(`/api/v1/sessions/${session_id}/generate_title`, data);
}

export async function getMessageList(data: { session_id: string; limit: number, created_at: string }) {
  if (data.created_at) {
    return get(`/api/v1/messages/${data.session_id}/load?before_time=${encodeURIComponent(data.created_at)}&limit=${data.limit}`);
  } else {
    return get(`/api/v1/messages/${data.session_id}/load?limit=${data.limit}`);
  }
}

export async function delSession(session_id: string) {
  return del(`/api/v1/sessions/${session_id}`);
}

export async function batchDelSessions(ids: string[]) {
  return del(`/api/v1/sessions/batch`, { ids });
}

export async function deleteAllSessions() {
  return del(`/api/v1/sessions/batch`, { delete_all: true });
}

export async function getSession(session_id: string) {
  return get(`/api/v1/sessions/${session_id}`);
}

export async function stopSession(session_id: string, message_id: string) {
  return post(`/api/v1/sessions/${session_id}/stop`, { message_id });
}

export async function clearSessionMessages(session_id: string) {
  return del(`/api/v1/sessions/${session_id}/messages`);
}

// Renames a session via the partial-update endpoint documented in §8 of the
// chat API reference. Body shape is `{ title }`; backend returns the full
// session record so callers can reconcile cached state.
export async function updateSessionTitle(session_id: string, title: string) {
  return put(`/api/v1/sessions/${session_id}`, { title });
}

// ===== 推荐问题（新接口 §9.1） =====
export interface SuggestedQuestionNew {
  id: string;
  question: string;
  answer_mode: 'generated' | 'custom';
  custom_answer: string;
  sort_order: number;
  created_at: string;
  updated_at: string;
}

export function getSuggestedQuestionsNew(params?: {
  knowledge_base_ids?: string[];
  knowledge_ids?: string[];
  tag_ids?: string[];
  limit?: number;
}) {
  const query = new URLSearchParams();
  if (params?.knowledge_base_ids?.length) query.set('knowledge_base_ids', params.knowledge_base_ids.join(','));
  if (params?.knowledge_ids?.length) query.set('knowledge_ids', params.knowledge_ids.join(','));
  if (params?.tag_ids?.length) query.set('tag_ids', params.tag_ids.join(','));
  if (params?.limit) query.set('limit', String(params.limit));
  const qs = query.toString();
  return get<{ data: { questions: SuggestedQuestionNew[] } }>(
    `/api/v1/suggested-questions${qs ? '?' + qs : ''}`
  );
}
