/**
 * 消息反馈（点赞 / 点踩）相关接口占位
 *
 * TODO: 后端反馈接口尚未提供。当前所有函数为占位实现，仅在控制台
 *       输出用户反馈，并返回一个模拟的成功响应。
 *
 *       待后端 `/api/v1/messages/:id/feedback` 或类似端点 ready 后，
 *       把下面 `TODO` 标记的 fetch 逻辑替换成真实请求即可，
 *       调用方代码无需改动。
 *
 *       期望接口约定（待后端确认）：
 *         - POST /api/v1/messages/:message_id/feedback
 *         - body: {
 *             rating: 'like' | 'dislike',
 *             reason?: string,        // 选中项 key（点踩时必填）
 *             comment?: string,       // 自由文本（选择「其他」时必填）
 *           }
 *         - response: { success: boolean, message?: string }
 */

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

export interface SubmitFeedbackParams {
  message_id: string
  rating: FeedbackRating
  reason?: DislikeReasonKey | null
  comment?: string
}

export interface FeedbackSubmitResult {
  /** 是否成功；当前占位实现始终为 true */
  success: boolean
  message?: string
}

/**
 * 上报点赞 / 点踩反馈。
 *
 * 当前为占位实现：仅打印日志 + 模拟 200ms 延时后返回成功。
 * 后端接口 ready 后，请把函数体内的 `// TODO(...)` 段替换为真实
 * `post(...)` 调用（参考同目录 `index.ts` 中的 `pinSession` 等）。
 */
export async function submitMessageFeedback(
  params: SubmitFeedbackParams,
): Promise<FeedbackSubmitResult> {
  // eslint-disable-next-line no-console
  console.info('[feedback:stub] submitMessageFeedback', params)

  // TODO(backend-pending): 替换为真实接口调用
  //   import { post } from '../../utils/request'
  //   return post<FeedbackSubmitResult>(
  //     `/api/v1/messages/${params.message_id}/feedback`,
  //     {
  //       rating: params.rating,
  //       reason: params.reason ?? undefined,
  //       comment: params.comment ?? undefined,
  //     },
  //   )

  await new Promise((resolve) => setTimeout(resolve, 200))

  return { success: true, message: 'ok (stub)' }
}