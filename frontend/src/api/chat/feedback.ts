/**
 * 消息反馈（点赞 / 点踩）接口封装
 *
 * 后端约定（见 docs/api/chat相关接口文档.md §6）：
 *   - PUT    /api/v1/messages/{session_id}/{message_id}/feedback  提交 / 覆盖反馈
 *   - DELETE /api/v1/messages/{session_id}/{message_id}/feedback  取消反馈
 *   - body（PUT）: { rating: 'like' | 'dislike', reason?: string, comment?: string }
 *   - 响应 data（PUT）: FeedbackRecord
 *
 * reason / comment 仅 dislike 时有意义；后端会维护 code ↔ 多语言映射，
 * 前端按 key 上报即可。
 */

import { put, del } from '@/utils/request'

export type FeedbackRating = 'like' | 'dislike'

/**
 * 点踩原因枚举的 key。前端按这个 key 上报，
 * 后端再决定是存 key、还是翻译后再落库。
 */
export type DislikeReasonKey =
  | 'factual_error'
  | 'logic_confusion'
  | 'outdated'
  | 'format_error'
  | 'too_long'
  | 'repetitive'
  | 'other'

/** 后端返回的反馈记录（成功响应 data 字段）。 */
export interface FeedbackRecord {
  id: string
  message_id: string
  session_id: string
  rating: FeedbackRating
  reason: DislikeReasonKey | null
  /** 后端翻译好的中文标签，前端可直接展示 */
  reason_zh?: string
  /** 后端翻译好的英文标签，前端可直接展示 */
  reason_en?: string
  comment: string | null
  created_at: string
  updated_at: string
}

export interface SubmitFeedbackParams {
  /** 必填，所属会话 id（接口路径参数） */
  session_id: string
  /** 必填，目标消息 id（接口路径参数） */
  message_id: string
  rating: FeedbackRating
  /** dislike 时可选；后端存 key，不在前端翻译 */
  reason?: DislikeReasonKey | null
  /** dislike + 选了「其他」时建议必填 */
  comment?: string
}

export interface FeedbackSubmitResult {
  success: boolean
  /** 成功时由后端返回的反馈记录；用于前端状态回显与样式驱动 */
  data?: FeedbackRecord
  message?: string
}

/** 取消反馈的入参；无需 rating / reason */
export interface CancelFeedbackParams {
  session_id: string
  message_id: string
}

const buildFeedbackPath = (sessionId: string, messageId: string): string =>
  `/api/v1/messages/${encodeURIComponent(sessionId)}/${encodeURIComponent(messageId)}/feedback`

/**
 * 上报点赞 / 点踩反馈。
 * 后端按 PUT 语义：再次提交会覆盖前一次的反馈。
 */
export async function submitMessageFeedback(
  params: SubmitFeedbackParams,
): Promise<FeedbackSubmitResult> {
  const { session_id, message_id, rating, reason, comment } = params
  if (!session_id || !message_id) {
    return { success: false, message: 'session_id / message_id 不能为空' }
  }
  const body: Record<string, unknown> = { rating }
  if (rating === 'dislike') {
    if (reason) body.reason = reason
    if (comment) body.comment = comment
  }
  try {
    const data = await put<FeedbackRecord>(buildFeedbackPath(session_id, message_id), body)
    return { success: true, data }
  } catch (err: any) {
    return { success: false, message: err?.message || '提交反馈失败' }
  }
}

/**
 * 取消点赞 / 点踩（DELETE 语义）。
 * 成功后不会回传 FeedbackRecord，由调用方自行清空本地状态。
 */
export async function cancelMessageFeedback(
  params: CancelFeedbackParams,
): Promise<FeedbackSubmitResult> {
  const { session_id, message_id } = params
  if (!session_id || !message_id) {
    return { success: false, message: 'session_id / message_id 不能为空' }
  }
  try {
    await del(buildFeedbackPath(session_id, message_id))
    return { success: true }
  } catch (err: any) {
    return { success: false, message: err?.message || '取消反馈失败' }
  }
}
