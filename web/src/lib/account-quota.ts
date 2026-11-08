import type { Account, QuotaSnapshot, QuotaWindow } from './types'

export function freshQuota(snapshot?: QuotaSnapshot, now = Date.now()): snapshot is QuotaSnapshot {
  return !!snapshot && !snapshot.error && !snapshot.stale && snapshot.read_at > 0 &&
    now - snapshot.read_at * 1000 <= 15 * 60_000 && snapshot.read_at * 1000 <= now + 60_000 &&
    // A reset or package expiry can invalidate both a component and its total.
    // Keep the whole reading historical until refreshed; do not guess a new balance.
    (snapshot.windows || []).every((w) => [w.resets_at, w.expires_at].every((at) =>
      !at || Date.parse(at) > now))
}

export function knownWindow(w: QuotaWindow): boolean {
  return Number.isFinite(w.limit) && w.limit > 0 && Number.isFinite(w.remaining) && w.remaining >= 0
}

/** Same service + same named window + same unit only. Never add Total to its parts.
 * Only currently ready accounts contribute; missing/stale data is coverage, not zero.
 * Do not use for shared-wallet custom keys without a billing-scope identity.
 */
export function aggregateQuota(accounts: Account[], now = Date.now()) {
  const windows = new Map<string, QuotaWindow & { accounts: number }>()
  let covered = 0
  const ready = accounts.filter((a) => a.enabled && a.live?.ready)
  for (const a of ready) {
    const snapshot = a.live?.quota_snapshot
    if (!freshQuota(snapshot, now)) continue
    const valid = snapshot.windows?.filter(knownWindow) || []
    if (!valid.length) continue
    covered++
    for (const w of valid) {
      const key = JSON.stringify([w.id, w.unit, !!w.aside])
      const previous = windows.get(key)
      if (previous) {
        previous.limit += w.limit
        previous.remaining += Math.min(w.limit, w.remaining)
        previous.accounts++
      } else {
        windows.set(key, { ...w, remaining: Math.min(w.limit, w.remaining), accounts: 1 })
      }
    }
  }
  return { windows: [...windows.values()], covered, eligible: ready.length }
}

export function quotaLabel(name: string): string {
  return ({ Credits: '套餐积分', 'Extra credits': '额外积分', 'Total credits': '积分合计' } as Record<string, string>)[name] || name
}
