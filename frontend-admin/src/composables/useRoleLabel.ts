import { useI18n } from 'vue-i18n'

export function useRoleLabel() {
  const { t } = useI18n()
  const formatRole = (role: string | null | undefined): string => {
    if (!role) return ''
    const key = `userRole.role.${role}`
    const label = t(key)
    return label === key ? role : label
  }
  const icons: Record<string, string> = {
    system_admin: 'server',
    knowledge_domain_admin: 'user-circle',
    viewer: 'browse',
  }
  const roleIcon = (role: string | null | undefined): string =>
    (role && icons[role]) || ''
  return { formatRole, roleIcon }
}
