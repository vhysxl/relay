export function formatTimestamp(isoStr?: string | null): string {
  if (!isoStr) return '—'
  const d = new Date(isoStr)
  if (isNaN(d.getTime())) return '—'
  const pad = (n: number) => String(n).padStart(2, '0')
  const year = d.getFullYear()
  const month = pad(d.getMonth() + 1)
  const day = pad(d.getDate())
  const hours = pad(d.getHours())
  const minutes = pad(d.getMinutes())
  const seconds = pad(d.getSeconds())
  return `${year}-${month}-${day} ${hours}:${minutes}:${seconds}`
}

export function formatDuration(startStr?: string | null, endStr?: string | null): string {
  if (!startStr || !endStr) return '—'
  const start = new Date(startStr)
  const end = new Date(endStr)
  const diffMs = end.getTime() - start.getTime()
  if (isNaN(diffMs) || diffMs < 0) return '—'
  if (diffMs === 0) return '< 1ms'
  if (diffMs < 1000) return `${diffMs}ms`
  const sec = (diffMs / 1000).toFixed(2)
  if (Number(sec) < 60) return `${sec}s`
  const min = Math.floor(Number(sec) / 60)
  const remSec = (Number(sec) % 60).toFixed(0)
  return `${min}m ${remSec}s`
}

export function formatMs(ms: number): string {
  if (isNaN(ms) || ms < 0) return '—'
  if (ms === 0) return '< 1ms'
  if (ms < 1000) return `${Math.round(ms)}ms`
  if (ms < 60000) {
    const s = (ms / 1000).toFixed(ms % 1000 === 0 ? 0 : 2)
    return `${s}s`
  }
  const min = Math.floor(ms / 60000)
  const remS = ((ms % 60000) / 1000).toFixed(0)
  return `${min}m ${remS}s`
}

export function formatJSON(data: any): string {
  if (!data) return ''
  try {
    const parsed = typeof data === 'string' ? JSON.parse(data) : data
    return JSON.stringify(parsed, null, 2)
  } catch {
    return String(data)
  }
}

export function formatUptime(seconds?: number): string {
  if (seconds === undefined || seconds === null || seconds < 0) return '—'
  if (seconds < 60) return `${seconds}s`
  const min = Math.floor(seconds / 60)
  const remSec = seconds % 60
  if (min < 60) return `${min}m ${remSec}s`
  const hrs = Math.floor(min / 60)
  const remMin = min % 60
  if (hrs < 24) return `${hrs}h ${remMin}m`
  const days = Math.floor(hrs / 24)
  const remHrs = hrs % 24
  return `${days}d ${remHrs}h`
}
