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
import { useTranslation } from 'react-i18next'

import {
  Table,
  TableBody,
  TableCell,
  TableFooter,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { toIntlLocale } from '@/i18n/languages'
import { formatBillingCurrencyFromUSD } from '@/lib/currency'
import { formatLogQuota, formatNumber } from '@/lib/format'

import type { UsageLog } from '../../data/schema'
import { getTokenCharges } from '../../lib/charge-breakdown'
import type { LogOtherData } from '../../types'
import { DetailSection } from './log-detail-layout'

export function ChargeBreakdown(props: { log: UsageLog; other: LogOtherData }) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const lines = getTokenCharges(
    props.other,
    props.log.prompt_tokens,
    props.log.completion_tokens
  )
  const moneyOptions = { digitsLarge: 6, digitsSmall: 8, abbreviate: false }
  const quota =
    props.other.billing_source === 'subscription'
      ? (props.other.subscription_consumed ?? props.log.quota)
      : props.log.quota
  const known = lines.every(
    (line) => line.included || (line.tokens !== null && line.price !== null)
  )
  const subtotal = lines.reduce(
    (sum, line) => sum + ((line.tokens ?? 0) * (line.price ?? 0)) / 1_000_000,
    0
  )
  const units = props.other.billing_quota_per_unit
  const difference =
    known && typeof units === 'number' && units > 0
      ? quota / units - subtotal
      : null
  const total =
    typeof units === 'number' && units > 0
      ? formatBillingCurrencyFromUSD(quota / units, moneyOptions)
      : formatLogQuota(quota)

  return (
    <DetailSection label={t('Charge Breakdown')}>
      <Table
        aria-label={t('Charge Breakdown')}
        className='table-fixed [&_td]:text-xs [&_th]:text-xs'
      >
        <TableHeader>
          <TableRow>
            <TableHead className='w-[31%] px-1.5 whitespace-normal'>
              {t('Charge item')}
            </TableHead>
            <TableHead className='w-[19%] px-1.5 text-right'>
              {t('Tokens')}
            </TableHead>
            <TableHead className='w-[23%] px-1.5 text-right whitespace-normal'>
              {t('Unit price (per 1M tokens)')}
            </TableHead>
            <TableHead className='w-[27%] px-1.5 text-right whitespace-normal'>
              {t('Charged amount')}
            </TableHead>
          </TableRow>
        </TableHeader>
        <TableBody className='[&>tr]:h-auto'>
          {lines.map((line) => {
            let amount = t('Not reported')
            if (line.included) amount = t('Not charged separately')
            if (line.tokens !== null && line.price !== null) {
              amount = formatBillingCurrencyFromUSD(
                (line.tokens * line.price) / 1_000_000,
                moneyOptions
              )
            }
            const price =
              line.price === null
                ? t('Unit price unavailable')
                : formatBillingCurrencyFromUSD(line.price, moneyOptions)
            return (
              <TableRow key={line.key}>
                <TableHead
                  scope='row'
                  className='h-auto px-1.5 py-2 whitespace-normal'
                >
                  {t(line.label)}
                </TableHead>
                <TableCell className='px-1.5 text-right break-all whitespace-normal'>
                  {line.tokens === null
                    ? t('Not reported')
                    : formatNumber(line.tokens, locale)}
                </TableCell>
                <TableCell className='px-1.5 text-right break-all whitespace-normal'>
                  {price}
                </TableCell>
                <TableCell className='px-1.5 text-right font-semibold break-all whitespace-normal'>
                  {amount}
                </TableCell>
              </TableRow>
            )
          })}
          {difference !== null && Math.abs(difference) > 1e-10 && (
            <TableRow>
              <TableHead
                scope='row'
                colSpan={3}
                className='h-auto px-1.5 py-2 whitespace-normal'
              >
                {t('Other charges and rounding')}
              </TableHead>
              <TableCell className='px-1.5 text-right break-all whitespace-normal'>
                {formatBillingCurrencyFromUSD(difference, moneyOptions)}
              </TableCell>
            </TableRow>
          )}
        </TableBody>
        <TableFooter>
          <TableRow>
            <TableHead scope='row' colSpan={3} className='px-1.5'>
              {t('Total')}
            </TableHead>
            <TableCell className='px-1.5 text-right font-semibold break-all whitespace-normal'>
              {total}
            </TableCell>
          </TableRow>
        </TableFooter>
      </Table>
      <p className='text-muted-foreground pt-1 text-xs'>
        {t('Effective unit prices include group and request multipliers.')}
      </p>
    </DetailSection>
  )
}
