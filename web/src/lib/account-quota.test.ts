import { test } from 'node:test'
import assert from 'node:assert/strict'
import { aggregateQuota, freshQuota } from './account-quota.ts'
import type { Account, QuotaWindow } from './types.ts'

const now = 1_800_000_000_000
const window = (id = 'Total credits', unit = 'credits', aside = false): QuotaWindow => ({ id, name: id, unit, aside, limit: 100, remaining: 25 })
function account(id: number, windows: QuotaWindow[] = [window()], extra = {}): Account {
  return { id, engine: 'workbuddy', label: `account-${id}`, enabled: true, status: 'ok', detail: '', created_at: 0, updated_at: 0,
    live: { id: String(id), label: '', enabled: true, ready: true, status: 'ok', models: 1,
      quota_snapshot: { read_at: now / 1000, windows, ...extra } } }
}

test('same windows aggregate, totals and component credits remain separate', () => {
  const result = aggregateQuota([account(1, [window(), window('Credits', 'credits', true)]), account(2)], now)
  assert.equal(result.covered, 2)
  assert.equal(result.windows.length, 2)
  assert.equal(result.windows[0].remaining, 50)
  assert.equal(result.windows[0].limit, 200)
  assert.equal(result.windows[1].remaining, 25)
})

test('different units are not added', () => {
  const result = aggregateQuota([account(1), account(2, [window('Total credits', 'tokens')])], now)
  assert.equal(result.windows.length, 2)
  assert.deepEqual(result.windows.map((w) => w.remaining), [25, 25])
})

test('unknown, failed, stale and disabled do not become zero balances', () => {
  const off = account(5); off.enabled = false
  const cooling = account(6); cooling.live.ready = false
  const unknown = account(7); delete unknown.live.quota_snapshot
  const result = aggregateQuota([account(1), account(2, [], { error: 'offline' }), account(3, [window()], { stale: true }), account(4, [window()], { read_at: (now - 16 * 60_000) / 1000 }), off, cooling, unknown], now)
  assert.equal(result.covered, 1)
  assert.equal(result.eligible, 5)
  assert.equal(result.windows[0].remaining, 25)
})

test('reset and package expiry invalidate recent readings including their totals', () => {
  for (const field of ['resets_at', 'expires_at'] as const) {
    for (const boundary of [now - 1, now]) {
      const before = account(1, [
        { ...window('Extra credits', 'credits', true), [field]: new Date(boundary).toISOString() },
        window(),
      ])
      assert.equal(freshQuota(before.live.quota_snapshot, now), false)
      const result = aggregateQuota([before, account(2)], now)
      assert.equal(result.covered, 1)
      assert.equal(result.eligible, 2)
      assert.equal(result.windows.length, 1)
      assert.equal(result.windows[0].remaining, 25)
      // Keep historical values intact for account detail, not a fabricated zero.
      assert.equal(before.live.quota_snapshot?.windows?.[1].remaining, 25)
    }
    const upcoming = account(3, [{ ...window(), [field]: new Date(now + 1).toISOString() }])
    assert.equal(freshQuota(upcoming.live.quota_snapshot, now), true)
    assert.equal(freshQuota(upcoming.live.quota_snapshot, now + 1), false)
    assert.equal(freshQuota(account(4, [{ ...window(), [field]: 'invalid' }]).live.quota_snapshot, now), false)
    assert.equal(freshQuota(account(5, [{ ...window(), [field]: '' }]).live.quota_snapshot, now), true)
  }
})

test('invalid or future samples are not trusted', () => {
  assert.equal(freshQuota({ read_at: 0 }, now), false)
  assert.equal(freshQuota({ read_at: (now + 2 * 60_000) / 1000 }, now), false)
  assert.equal(aggregateQuota([account(1, [{ ...window(), limit: NaN }])], now).covered, 0)
})
