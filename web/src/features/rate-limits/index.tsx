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
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { StaticDataTable, StaticRowActions } from '@/components/data-table'
import { ErrorState } from '@/components/error-state'
import { SectionPageLayout } from '@/components/layout'
import { LoadingState } from '@/components/loading-state'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Combobox } from '@/components/ui/combobox'
import { Input } from '@/components/ui/input'
import { toIntlLocale } from '@/i18n/languages'
import { formatNumber } from '@/lib/format'

import { getLimits, getUsage, saveLimits, type Config, type Rule } from './api'
import { RuleDialog } from './rule-dialog'

export function RateLimits() {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const client = useQueryClient()
  const query = useQuery({
    queryKey: ['gateway-limits', 'config'],
    queryFn: getLimits,
    staleTime: 30_000,
  })
  const [scope, setScope] = useState('all')
  const [search, setSearch] = useState('')
  const [page, setPage] = useState(0)
  const [editor, setEditor] = useState<Rule | 'new' | null>(null)
  const [deleting, setDeleting] = useState<Rule | null>(null)
  const filtered = (query.data?.config.rules ?? []).filter(
    (rule) =>
      (scope === 'all' || rule.scope === scope) &&
      `${rule.name} ${rule.subject_id} ${rule.model} ${rule.channel_ids?.join(' ')}`
        .toLowerCase()
        .includes(search.toLowerCase())
  )
  const currentPage = Math.min(
    page,
    Math.max(0, Math.ceil(filtered.length / 50) - 1)
  )
  const visible = filtered.slice(currentPage * 50, currentPage * 50 + 50)
  const ids = visible.map((rule) => rule.id)
  const usage = useQuery({
    queryKey: ['gateway-limits', 'usage', ids],
    queryFn: () => getUsage(ids),
    enabled: ids.length > 0,
    refetchInterval: 10_000,
    refetchIntervalInBackground: false,
    retry: false,
  })
  const current = new Map(usage.data?.map((value) => [value.id, value]))
  const mutation = useMutation({
    mutationFn: saveLimits,
    onSuccess: (settings) => {
      client.setQueryData(['gateway-limits', 'config'], settings)
      void client.invalidateQueries({ queryKey: ['gateway-limits', 'usage'] })
    },
  })
  async function persist(rules: Rule[]): Promise<void> {
    if (!query.data) return
    const config: Config = { revision: query.data.config.revision, rules }
    await mutation.mutateAsync(config)
    setEditor(null)
    setDeleting(null)
  }
  const scopeLabels: Record<string, string> = {
    all: t('All'),
    user: t('Users'),
    api_key: t('API Keys'),
    user_model: t('User + model'),
    provider: t('Supplier pool'),
  }
  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>
        {t('Gateway rate limits')}
      </SectionPageLayout.Title>
      <SectionPageLayout.Content>
        {query.isPending && <LoadingState />}
        {query.isError && <ErrorState onRetry={() => void query.refetch()} />}
        {query.data && (
          <div className='space-y-4'>
            <div className='flex flex-wrap items-center gap-2'>
              <Badge variant='outline'>
                {query.data.backend === 'redis'
                  ? t('Shared across instances')
                  : t('Per-instance limits')}
              </Badge>
              <Button
                variant='outline'
                onClick={() => void query.refetch()}
                disabled={mutation.isPending}
              >
                {t('Refresh')}
              </Button>
              <Button
                onClick={() => setEditor('new')}
                disabled={mutation.isPending}
              >
                {t('Add rate limit')}
              </Button>
            </div>
            <div className='text-muted-foreground space-y-1 text-sm'>
              <p>
                {t(
                  'RPM refills continuously and allows a burst up to the configured limit.'
                )}
              </p>
              <p>
                {t(
                  'TPM reserves estimated input, then settles actual usage. In-flight output may temporarily exceed the limit.'
                )}
              </p>
              <p>
                {t(
                  'Supplier channels in one pool share limits. Task concurrency covers submission, not background execution.'
                )}
              </p>
              {query.data.legacy_enabled && (
                <p className='text-warning'>
                  {t(
                    'Legacy model rate limits are also enabled and apply independently.'
                  )}
                </p>
              )}
            </div>
            <div className='flex flex-col gap-2 sm:flex-row'>
              <Combobox
                aria-label={t('Scope')}
                className='sm:w-56'
                value={scope}
                options={Object.entries(scopeLabels).map(([value, label]) => ({
                  value,
                  label,
                }))}
                onValueChange={(value) => {
                  setScope(value ?? 'all')
                  setPage(0)
                }}
              />
              <Input
                aria-label={t('Search')}
                placeholder={t('Search')}
                value={search}
                onChange={(event) => {
                  setSearch(event.target.value)
                  setPage(0)
                }}
              />
            </div>
            {usage.isError && (
              <p role='alert' className='text-destructive text-sm'>
                {t('Live usage is unavailable. Refresh to retry.')}
              </p>
            )}
            <StaticDataTable
              data={visible}
              getRowKey={(rule) => rule.id}
              emptyContent={t('No rate limit rules')}
              columns={[
                {
                  id: 'name',
                  header: t('Name'),
                  cell: (rule) => (
                    <div className='max-w-64 break-words'>
                      {rule.name}
                      <div className='text-muted-foreground text-xs'>
                        {scopeLabels[rule.scope]} ·{' '}
                        {rule.scope === 'provider'
                          ? rule.channel_ids.map((id) => `#${id}`).join(', ')
                          : `#${rule.subject_id} ${rule.model}`}
                      </div>
                    </div>
                  ),
                },
                ...(['rpm', 'tpm', 'concurrent'] as const).map((dimension) => ({
                  id: dimension,
                  header:
                    dimension === 'concurrent'
                      ? t('Concurrency')
                      : dimension.toUpperCase(),
                  cell: (rule: Rule) => (
                    <span className='whitespace-nowrap tabular-nums'>
                      {usage.isError || !current.has(rule.id)
                        ? '—'
                        : formatNumber(
                            current.get(rule.id)?.[dimension] ?? 0,
                            locale
                          )}{' '}
                      /{' '}
                      {rule[dimension] === 0
                        ? t('Unlimited')
                        : formatNumber(rule[dimension], locale)}
                    </span>
                  ),
                })),
                {
                  id: 'status',
                  header: t('Status'),
                  cell: (rule) => (
                    <Badge variant={rule.enabled ? 'default' : 'outline'}>
                      {rule.enabled ? t('Enabled') : t('Disabled')}
                    </Badge>
                  ),
                },
                {
                  id: 'actions',
                  header: t('Actions'),
                  cell: (rule) => (
                    <StaticRowActions
                      editDisabled={mutation.isPending}
                      deleteDisabled={mutation.isPending}
                      onEdit={() => setEditor(rule)}
                      onDelete={() => setDeleting(rule)}
                      editLabel={t('Edit')}
                      deleteLabel={t('Delete')}
                      menuLabel={t('Open menu')}
                    />
                  ),
                },
              ]}
            />
            <div className='flex items-center justify-end gap-3'>
              <span className='text-muted-foreground text-sm'>
                {formatNumber(filtered.length, locale)} ·{' '}
                {formatNumber(currentPage + 1, locale)} /{' '}
                {formatNumber(
                  Math.max(1, Math.ceil(filtered.length / 50)),
                  locale
                )}
              </span>
              <Button
                variant='outline'
                disabled={currentPage === 0}
                onClick={() => setPage(currentPage - 1)}
              >
                {t('Previous')}
              </Button>
              <Button
                variant='outline'
                disabled={(currentPage + 1) * 50 >= filtered.length}
                onClick={() => setPage(currentPage + 1)}
              >
                {t('Next')}
              </Button>
            </div>
            {editor && (
              <RuleDialog
                key={editor === 'new' ? 'new' : editor.id}
                rule={editor === 'new' ? null : editor}
                pending={mutation.isPending}
                onClose={() => setEditor(null)}
                onSave={async (rule) => {
                  const rules = query.data.config.rules
                  await persist(
                    editor === 'new'
                      ? [...rules, rule]
                      : rules.map((existing) =>
                          existing.id === rule.id ? rule : existing
                        )
                  )
                }}
              />
            )}
            <ConfirmDialog
              open={!!deleting}
              onOpenChange={(open) => {
                if (!open && !mutation.isPending) setDeleting(null)
              }}
              title={t('Delete rate limit')}
              desc={deleting?.name ?? ''}
              destructive
              isLoading={mutation.isPending}
              handleConfirm={() => {
                void persist(
                  query.data.config.rules.filter(
                    (rule) => rule.id !== deleting?.id
                  )
                ).catch(() => {})
              }}
            />
          </div>
        )}
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
