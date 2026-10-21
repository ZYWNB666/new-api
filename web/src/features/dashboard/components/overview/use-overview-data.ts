/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { useQuery } from '@tanstack/react-query'

import { getUserQuotaDates } from '@/features/dashboard/api'
import type { QuotaDataItem } from '@/features/dashboard/types'
import { getPerfMetricsSummary } from '@/features/performance-metrics/api'
import { getUserLogs } from '@/features/usage-logs/api'
import type { UsageLog } from '@/features/usage-logs/data/schema'
import { requireServerSuccess } from '@/lib/server-error-message'
import { computeTimeRange, toStartOfDay } from '@/lib/time'

export type TrendMetric = 'quota' | 'requests' | 'tokens'
export type TrendRange = 7 | 30

export interface TrendPoint {
  label: string
  quota: number
  requests: number
  tokens: number
}

export interface ModelRank {
  name: string
  quota: number
  requests: number
  tokens: number
}

export interface OverviewData {
  todayQuota: number
  todayRequests: number
  todayTokens: number
  trend: TrendPoint[]
  topModels: ModelRank[]
  modelQuotaShare: ModelRank[]
  recentCalls: UsageLog[]
  platformSuccessRate: number | null
  loading: boolean
}

const DAY = 24 * 3600

function dayKey(ts: number): number {
  return toStartOfDay(ts)
}

function buildTrend(data: QuotaDataItem[], days: TrendRange): TrendPoint[] {
  const now = Math.floor(Date.now() / 1000)
  const todayStart = dayKey(now)
  const buckets = new Map<number, TrendPoint>()
  for (let i = days - 1; i >= 0; i--) {
    const key = todayStart - i * DAY
    const d = new Date(key * 1000)
    buckets.set(key, {
      label: `${d.getMonth() + 1}/${d.getDate()}`,
      quota: 0,
      requests: 0,
      tokens: 0,
    })
  }
  for (const item of data) {
    const key = dayKey(Number(item.created_at))
    const bucket = buckets.get(key)
    if (!bucket) continue
    bucket.quota += Number(item.quota) || 0
    bucket.requests += Number(item.count) || 0
    bucket.tokens += Number(item.token_used) || 0
  }
  return [...buckets.values()]
}

function aggregateByModel(data: QuotaDataItem[]): ModelRank[] {
  const map = new Map<string, ModelRank>()
  for (const item of data) {
    const name = item.model_name || ''
    if (!name) continue
    const entry = map.get(name) ?? { name, quota: 0, requests: 0, tokens: 0 }
    entry.quota += Number(item.quota) || 0
    entry.requests += Number(item.count) || 0
    entry.tokens += Number(item.token_used) || 0
    map.set(name, entry)
  }
  return [...map.values()]
}

export function useOverviewData(trendRange: TrendRange): OverviewData {
  const range = computeTimeRange(trendRange)
  const quotaQuery = useQuery({
    queryKey: ['dashboard', 'overview', 'usage', range.start_timestamp, range.end_timestamp],
    queryFn: async () =>
      requireServerSuccess(await getUserQuotaDates(range)).data ?? [],
    staleTime: 60 * 1000,
  })

  const logsQuery = useQuery({
    queryKey: ['dashboard', 'overview', 'recent-calls'],
    queryFn: async () => {
      const res = await getUserLogs({ p: 1, page_size: 5 })
      return (res?.data?.items ?? []) as UsageLog[]
    },
    staleTime: 30 * 1000,
  })

  const perfQuery = useQuery({
    queryKey: ['dashboard', 'overview', 'success-rate'],
    queryFn: async () => {
      const res = await getPerfMetricsSummary(24)
      return res?.data?.summary?.success_rate ?? null
    },
    staleTime: 60 * 1000,
    retry: false,
  })

  const rawData = quotaQuery.data
  const data: QuotaDataItem[] = Array.isArray(rawData) ? rawData : []
  const todayStart = dayKey(Math.floor(Date.now() / 1000))
  let todayQuota = 0
  let todayRequests = 0
  let todayTokens = 0
  for (const item of data) {
    if (dayKey(Number(item.created_at)) === todayStart) {
      todayQuota += Number(item.quota) || 0
      todayRequests += Number(item.count) || 0
      todayTokens += Number(item.token_used) || 0
    }
  }

  const byModel = aggregateByModel(data)
  const topByRequests = [...byModel]
    .sort((a, b) => b.requests - a.requests)
    .slice(0, 5)
  const topByQuota = [...byModel]
    .sort((a, b) => b.quota - a.quota)
    .slice(0, 6)

  return {
    todayQuota,
    todayRequests,
    todayTokens,
    trend: buildTrend(data, trendRange),
    topModels: topByRequests,
    modelQuotaShare: topByQuota,
    recentCalls: logsQuery.data ?? [],
    platformSuccessRate: perfQuery.data ?? null,
    loading: quotaQuery.isLoading,
  }
}
