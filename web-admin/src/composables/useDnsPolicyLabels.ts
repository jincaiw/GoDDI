import type { ComposerTranslation } from 'vue-i18n';

export function dnsMatchLabel(t: ComposerTranslation, value: string): string {
  const key = `dns.security.matchValues.${String(value).toLowerCase()}`;
  const out = t(key);
  return out === key ? value : out;
}

export function dnsResponseLabel(t: ComposerTranslation, value: string): string {
  const key = `dns.security.responseValues.${String(value).toLowerCase()}`;
  const out = t(key);
  return out === key ? value : out;
}

export function dnsListTypeLabel(t: ComposerTranslation, value: string): string {
  const key = `dns.security.listTypes.${String(value).toLowerCase()}`;
  const out = t(key);
  return out === key ? value : out;
}
