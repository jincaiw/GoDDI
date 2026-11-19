import { $t } from '@/locales';

/**
 * Build a delete confirmation body that names the exact resource.
 * Operators must not be asked to confirm an anonymous deletion; a human-readable
 * name is strongly preferred over an opaque ID. Pass the name when the row has
 * one, and the ID only as a fallback identifier.
 */
export function namedDeleteBody(identifier: string | number | null | undefined): string {
  const base = $t('common.deleteConfirm');
  const label = identifier === null || identifier === undefined ? '' : String(identifier).trim();
  return label ? `${base}：${label}` : base;
}

/**
 * Prefer a readable name, falling back to the ID when the name is absent.
 * Usage: `namedDeleteBody(rowLabel(row))`.
 */
export function rowLabel(row: Record<string, unknown>, ...candidates: string[]): string {
  for (const key of candidates) {
    const value = row[key];
    if (typeof value === 'string' && value.trim()) return value.trim();
    if (typeof value === 'number') return String(value);
  }
  const fallback = row.id ?? row.name ?? '';
  return String(fallback);
}
