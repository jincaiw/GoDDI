import type { ComposerTranslation } from 'vue-i18n';

export function dhcpLeaseStatusLabel(t: ComposerTranslation, value: string): string {
  const key = `dhcp.leases.statusValues.${String(value).toLowerCase()}`;
  const out = t(key);
  return out === key ? value : out;
}

export function haStateLabel(t: ComposerTranslation, value: string): string {
  const key = `haStatus.stateValues.${String(value).toLowerCase()}`;
  const out = t(key);
  return out === key ? value : out;
}

export function haRoleLabel(t: ComposerTranslation, value: string): string {
  const key = `haStatus.roleValues.${String(value).toLowerCase()}`;
  const out = t(key);
  return out === key ? value : out;
}
