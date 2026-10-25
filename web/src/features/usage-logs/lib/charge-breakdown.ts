import type { LogOtherData } from '../types'
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
import { getTieredBillingSummary } from './format'

export interface TokenCharge {
  key: string
  label: string
  tokens: number | null
  price: number | null
  included: boolean
}

function validNumber(value: unknown): value is number {
  return typeof value === 'number' && Number.isFinite(value) && value >= 0
}

// Only frozen quantities and prices can explain a historical charge. Missing
// unit prices are not replaced with today's model configuration.
export function getTokenCharges(
  other: LogOtherData,
  input: number,
  output: number
): TokenCharge[] {
  const tokens = other.billing_charge_tokens ?? other.billing_tokens
  let prices = other.billing_prices
  let expression = false
  if (other.billing_mode === 'tiered_expr') {
    prices = undefined
    const summary = getTieredBillingSummary(other)
    const group = other.billing_group_ratio
    if (
      summary &&
      summary.tier.billingUnit !== 'request' &&
      validNumber(group) &&
      tokens
    ) {
      let multiplier = group
      for (const rule of other.request_rules ?? []) {
        if (rule.matched && validNumber(rule.multiplier)) {
          multiplier *= rule.multiplier
        }
      }
      if (Number.isFinite(multiplier)) {
        prices = {}
        for (const [key, field] of Object.entries({
          p: 'inputPrice',
          c: 'outputPrice',
          cr: 'cacheReadPrice',
          cc: 'cacheCreatePrice',
          cc1h: 'cacheCreate1hPrice',
        })) {
          const price = summary.tier[field]
          if (validNumber(price)) prices[key] = price * multiplier
        }
        expression = true
      }
    }
  }

  let generic = tokens?.cc ?? other.cache_creation_tokens ?? 0
  let short = tokens?.cc5m ?? 0
  let long = tokens?.cc1h ?? 0
  let shortPrice = prices?.cc5m ?? prices?.cc
  let longPrice = prices?.cc1h
  const reportedShort = other.cache_creation_tokens_5m
  const reportedLong = other.cache_creation_tokens_1h
  const observedShort = reportedShort ?? 0
  const observedLong = reportedLong ?? 0
  // Split a generic billed cache-write quantity only when supplier evidence
  // agrees with it. Kimi TTL headers do not change the expression's unit price.
  if (
    validNumber(generic) &&
    validNumber(short) &&
    validNumber(long) &&
    validNumber(observedShort) &&
    validNumber(observedLong) &&
    (reportedShort !== undefined || reportedLong !== undefined) &&
    generic + short + long >= observedShort + observedLong
  ) {
    if (short === 0 && observedShort > 0 && generic >= observedShort) {
      short = observedShort
      generic -= short
      shortPrice = prices?.cc
    }
    if (long === 0 && observedLong > 0 && generic >= observedLong) {
      long = observedLong
      generic -= long
      longPrice = prices?.cc
    }
  }

  const items: Array<[string, string, unknown, unknown]> = [
    ['p', 'Input charge', tokens?.p ?? input, prices?.p],
    ['c', 'Output charge', tokens?.c ?? output, prices?.c],
    [
      'cc5m',
      'Cache write charge (5m)',
      reportedShort === undefined && short === 0 ? null : short,
      shortPrice,
    ],
    [
      'cc1h',
      'Cache write charge (1h)',
      reportedLong === undefined && long === 0 ? null : long,
      longPrice,
    ],
    [
      'cr',
      'Cache read charge',
      tokens?.cr ?? other.cache_tokens ?? 0,
      prices?.cr,
    ],
  ]
  if (validNumber(generic) && generic > 0) {
    items.push(['cc', 'Cache write charge', generic, prices?.cc])
  }
  return items.map(([key, label, quantity, price]) => ({
    key,
    label,
    tokens: validNumber(quantity) ? quantity : null,
    price: validNumber(price) ? price : null,
    included: expression && price === undefined,
  }))
}
