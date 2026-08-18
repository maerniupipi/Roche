// 首页推荐问题（管理端 /platform/recommend-questions 页面专用 API 层）。
//
// 接口：
//   GET §9.1 /api/v1/suggested-questions       → 拉取当前生效的列表（含 id）
//   PUT §9.2 /api/v1/suggested-questions/config → 全量覆盖 1/2/3 三个槽位的配置

import { get, put } from '@/utils/request'

export type SuggestedQuestionAnswerMode = 'generated' | 'custom'

export type SuggestedQuestionSlot = 1 | 2 | 3

export interface SuggestedQuestion {
  id: string
  question: string
  answer_mode: SuggestedQuestionAnswerMode
  custom_answer: string
  sort_order: SuggestedQuestionSlot
  created_at?: string
  updated_at?: string
}

export interface SuggestedQuestionItemInput {
  id?: string
  question: string
  answer_mode: SuggestedQuestionAnswerMode
  custom_answer: string
  sort_order: SuggestedQuestionSlot
}

interface ListResponse<T> {
  success: boolean
  data?: T
  message?: string
}

interface SimpleResponse {
  success: boolean
  message?: string
}

export async function listSuggestedQuestions(): Promise<ListResponse<{ questions: SuggestedQuestion[] }>> {
  return await get('/api/v1/suggested-questions') as unknown as ListResponse<{ questions: SuggestedQuestion[] }>
}

export async function updateSuggestedQuestionsConfig(
  items: SuggestedQuestionItemInput[],
): Promise<SimpleResponse> {
  return await put(
    '/api/v1/suggested-questions/config',
    { items },
  ) as unknown as SimpleResponse
}
