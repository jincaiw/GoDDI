export type IpamLifecycle = 'active' | 'retired';

export function lifecycleLabel(t: (key: string) => string, state: string): string {
  if (state === 'active') return t('ipam.lifecycleActive');
  if (state === 'retired') return t('ipam.lifecycleRetired');
  return state;
}

export function lifecycleTheme(state: string): 'success' | 'default' {
  return state === 'active' ? 'success' : 'default';
}
/** Localize the authoritative IPAM allocation state, preserving unknown values. */
export function ipamStatusLabel(t: (key: any) => string, status: string): string {
  const keys: Record<string, string> = {
    used: 'ipam.addresses.statusUsed',
    available: 'ipam.addresses.statusFree',
    reserved: 'ipam.addresses.statusReserved',
    conflict: 'ipam.addresses.statusConflict'
  };
  const key = keys[status];
  return key ? t(key) : status;
}

export function ipamStatusTheme(status: string): 'success' | 'default' | 'warning' | 'danger' {
  const themes: Record<string, 'success' | 'default' | 'warning' | 'danger'> = {
    used: 'success',
    available: 'default',
    reserved: 'warning',
    conflict: 'danger'
  };
  return themes[status] || 'default';
}
