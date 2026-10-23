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
import { z } from 'zod'

import { api } from '@/lib/api'
import { requireServerSuccess } from '@/lib/server-error-message'

const limit = z.number().int().min(0).max(1_000_000_000)
export const ruleSchema = z
  .object({
    id: z.string().min(1),
    name: z.string().trim().min(1).max(128),
    scope: z.enum(['user', 'api_key', 'user_model', 'provider']),
    subject_id: z.number().int().min(0),
    model: z.string().trim().max(256),
    channel_ids: z.array(z.number().int().positive()).max(1000),
    rpm: limit,
    tpm: limit,
    concurrent: limit,
    enabled: z.boolean(),
  })
  .refine((r) => r.rpm + r.tpm + r.concurrent > 0, {
    message: 'Set at least one positive limit',
    path: ['rpm'],
  })
  .refine(
    (r) =>
      r.scope === 'provider' ? r.channel_ids.length > 0 : r.subject_id > 0,
    { message: 'Select a target', path: ['subject_id'] }
  )
  .refine((r) => r.scope !== 'user_model' || r.model.length > 0, {
    message: 'Model is required',
    path: ['model'],
  })

export type Rule = z.infer<typeof ruleSchema>
export type Config = { revision: number; rules: Rule[] }
export type Settings = {
  config: Config
  backend: 'redis' | 'memory'
  legacy_enabled: boolean
}
export type Usage = { id: string; rpm: number; tpm: number; concurrent: number }
export type Target = { id: number; name: string; user_id?: number }
const base = '/api/option/gateway_limits'
export async function getLimits(): Promise<Settings> {
  return requireServerSuccess(
    (await api.get<{ success: boolean; data: Settings }>(base)).data
  ).data
}
export async function saveLimits(config: Config): Promise<Settings> {
  return requireServerSuccess(
    (await api.put<{ success: boolean; data: Settings }>(base, config)).data
  ).data
}
export async function getUsage(ids: string[]): Promise<Usage[]> {
  return requireServerSuccess(
    (
      await api.get<{ success: boolean; data: Usage[] }>(`${base}/usage`, {
        params: { ids: ids.join(',') },
      })
    ).data
  ).data
}
export async function getTargets(
  kind: string,
  search: string
): Promise<Target[]> {
  return requireServerSuccess(
    (
      await api.get<{ success: boolean; data: Target[] }>(`${base}/targets`, {
        params: { kind, search },
      })
    ).data
  ).data
}
