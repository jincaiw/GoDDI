import { useAuthStore } from '@/stores/auth'

export function usePermission() {
  const authStore = useAuthStore()

  function can(resource: string, action: string): boolean {
    return authStore.hasPermission(resource, action)
  }

  function canAny(checks: Array<{ resource: string; action: string }>): boolean {
    return authStore.hasAnyPermission(checks)
  }

  function canRead(resource: string): boolean {
    return can(resource, 'read')
  }

  function canWrite(resource: string): boolean {
    return can(resource, 'write')
  }

  function canDelete(resource: string): boolean {
    return can(resource, 'delete')
  }

  return {
    can,
    canAny,
    canRead,
    canWrite,
    canDelete,
  }
}
