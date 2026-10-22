import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
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
import { describe, expect, test, vi } from 'vitest'

import { PaymentConfirmDialog } from '../components/dialogs/payment-confirm-dialog'
import { RechargeFormCard } from '../components/recharge-form-card'
import { PAYMENT_TYPES } from '../constants'
import { requestPaymentAmount } from '../hooks/use-payment'

describe('payment amount routing', () => {
  test('uses the dedicated Waffo amount calculator', async () => {
    const calls: string[] = []
    const amount = await requestPaymentAmount(120, PAYMENT_TYPES.WAFFO, {
      regular: async () => {
        calls.push('regular')
        return { success: true, data: '1' }
      },
      stripe: async () => {
        calls.push('stripe')
        return { success: true, data: '2' }
      },
      alipay: async () => {
        calls.push('alipay')
        return { success: true, data: '3' }
      },
      waffo: async (request) => {
        calls.push(`waffo:${request.amount}`)
        return { success: true, data: '18.75' }
      },
      waffoPancake: async () => {
        calls.push('pancake')
        return { success: true, data: '4' }
      },
    })

    expect(amount).toBe(18.75)
    expect(calls).toEqual(['waffo:120'])
  })
})

test.each([
  ['alipay', 'regular'],
  ['alipay_direct', 'alipay'],
])('routes %s payments through %s', async (paymentType, expected) => {
  const calls: string[] = []
  const calculator = (name: string) => async () => {
    calls.push(name)
    return { success: true, data: '7.30' }
  }
  const calculators = {
    regular: calculator('regular'),
    stripe: calculator('stripe'),
    alipay: calculator('alipay'),
    waffo: calculator('waffo'),
    waffoPancake: calculator('waffoPancake'),
  }
  expect(await requestPaymentAmount(1, paymentType, calculators)).toBe(7.3)
  expect(calls).toEqual([expected])
})

test('offers direct Alipay without Epay and confirms the charge in CNY', async () => {
  const selected = vi.fn()
  const method = {
    name: 'Alipay',
    type: PAYMENT_TYPES.ALIPAY_DIRECT,
    min_topup: 1,
  }
  const { unmount } = render(
    <RechargeFormCard
      topupInfo={{
        enable_alipay_topup: true,
        enable_online_topup: false,
        enable_stripe_topup: false,
        min_topup: 1,
        stripe_min_topup: 1,
        pay_methods: [method],
        amount_options: [],
        discount: {},
      }}
      presetAmounts={[]}
      selectedPreset={null}
      onSelectPreset={vi.fn()}
      topupAmount={1}
      onTopupAmountChange={vi.fn()}
      paymentAmount={7.3}
      calculating={false}
      onPaymentMethodSelect={selected}
      paymentLoading={null}
      redemptionCode=''
      onRedemptionCodeChange={vi.fn()}
      onRedeem={vi.fn()}
      redeeming={false}
      loading={false}
    />
  )
  const user = userEvent.setup()
  const button = screen.getByRole('button', { name: 'Alipay' })
  expect(button).toBeEnabled()
  await user.click(button)
  expect(selected).toHaveBeenCalledWith(method)
  unmount()
  render(
    <PaymentConfirmDialog
      open
      onOpenChange={vi.fn()}
      onConfirm={vi.fn()}
      topupAmount={1}
      paymentAmount={7.3}
      paymentMethod={method}
      calculating={false}
      processing={false}
    />
  )
  expect(screen.getByText('7.3 CNY')).toBeVisible()
})
