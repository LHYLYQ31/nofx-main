const SECONDS_PER_DAY = 86400

export function calculateBacktestDays(startTs?: number, endTs?: number): number | null {
  if (!Number.isFinite(startTs) || !Number.isFinite(endTs)) return null
  const start = Number(startTs)
  const end = Number(endTs)
  if (start <= 0 || end <= 0 || end < start) return null
  const spanSeconds = end - start
  const days = Math.ceil(spanSeconds / SECONDS_PER_DAY)
  return Math.max(1, days)
}

export function formatBacktestDays(startTs?: number, endTs?: number): string {
  const days = calculateBacktestDays(startTs, endTs)
  if (days === null) return '--d'
  return `${days}d`
}
