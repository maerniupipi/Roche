import { NotifyPlugin } from 'tdesign-vue-next'

type Translator = (key: string, params?: Record<string, unknown>) => string
type TemplateResolver = (key: string) => unknown

export function notifyLoginSuccess(
  _response: unknown,
  t: Translator,
  _tm?: TemplateResolver,
): void {
  NotifyPlugin.success({
    title: t('auth.loginSuccessTitle'),
    content: t('auth.loginSuccess'),
    duration: 4000,
    closeBtn: true,
  })
}
