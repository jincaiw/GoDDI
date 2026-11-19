/**
 * Format an API timestamp for display.
 * Go zero time serialises as 0001-01-01T00:00:00Z, which JavaScript
 * formats as 1/1/1 08:05:43 in Asia/Shanghai. Treat it as unknown.
 */
export function formatAPITime(value: string | null | undefined, empty = "-"): string {
  if (!value) return empty
  const text = String(value).trim()
  if (!text) return empty
  if (text.startsWith("0001-01-01") || text === "0000-00-00 00:00:00") return empty
  const date = new Date(text)
  if (Number.isNaN(date.getTime())) return empty
  return date.toLocaleString()
}
