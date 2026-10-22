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
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  act,
  fireEvent,
  render,
  renderHook,
  screen,
  waitFor,
} from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, test, vi, onTestFinished } from 'vitest'

import { api } from '@/lib/api'

import { AlipayPaymentDialog } from '../components/dialogs/alipay-payment-dialog'
import { PaymentConfirmDialog } from '../components/dialogs/payment-confirm-dialog'
import { RechargeFormCard } from '../components/recharge-form-card'
import { PAYMENT_TYPES } from '../constants'
import { requestPaymentAmount, usePayment } from '../hooks/use-payment'

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

test('keeps direct Alipay checkout in the wallet instead of navigating away', async () => {
  const checkout = {
    pay_link: 'https://openapi.alipay.com/gateway.do?sign=normal',
    qr_pay_link: 'https://openapi.alipay.com/gateway.do?sign=embedded',
    trade_no: 'alipay-embedded-test',
    expires_at: Math.floor(Date.now() / 1000) + 1800,
  }
  vi.spyOn(api, 'post').mockResolvedValue({
    data: { message: 'success', data: checkout },
  })
  const { result } = renderHook(() => usePayment())
  await act(async () => {
    expect(
      await result.current.processPayment(1, PAYMENT_TYPES.ALIPAY_DIRECT)
    ).toBe(true)
  })
  expect(result.current.alipayCheckout).toEqual(checkout)
})

const embeddedCheckout = {
  pay_link: 'https://openapi.alipay.com/gateway.do?sign=normal',
  qr_pay_link: 'https://openapi.alipay.com/gateway.do?sign=embedded',
  trade_no: 'alipay-qr-dialog',
}

test('shows the embedded QR checkout and reports success only after backend settlement', async () => {
  const client = new QueryClient()
  onTestFinished(() => client.clear())
  const get = vi
    .spyOn(api, 'get')
    .mockResolvedValue({ data: { success: true, data: { status: 'pending' } } })
  const onPaid = vi.fn()
  render(
    <QueryClientProvider client={client}>
      <AlipayPaymentDialog
        checkout={embeddedCheckout}
        onClose={vi.fn()}
        onPaid={onPaid}
      />
    </QueryClientProvider>
  )
  expect(await screen.findByText('Waiting for payment')).toBeVisible()
  const iframe = screen.getByTitle('Alipay payment QR code')
  expect(iframe).toHaveAttribute('src', embeddedCheckout.qr_pay_link)
  expect(
    screen.getByRole('button', { name: 'Open Alipay checkout' })
  ).toHaveAttribute('href', embeddedCheckout.pay_link)
  fireEvent.load(iframe)
  fireEvent(
    window,
    new MessageEvent('message', {
      origin: 'https://openapi.alipay.com',
      data: { status: 'success' },
    })
  )
  expect(onPaid).not.toHaveBeenCalled()
  get.mockResolvedValue({
    data: { success: true, data: { status: 'success' } },
  })
  await waitFor(() =>
    expect(
      screen.getByRole('button', { name: 'Check payment status' })
    ).toBeEnabled()
  )
  await userEvent
    .setup()
    .click(screen.getByRole('button', { name: 'Check payment status' }))
  expect(
    await screen.findByText('Payment received. Your balance has been updated.')
  ).toBeVisible()
  expect(screen.queryByTitle('Alipay payment QR code')).toBeNull()
  expect(onPaid).toHaveBeenCalledTimes(1)
  expect(get).toHaveBeenCalledWith(
    '/api/user/alipay/orders/alipay-qr-dialog',
    expect.objectContaining({ signal: expect.any(AbortSignal) })
  )
})

test('keeps a status query failure recoverable and closes with Escape', async () => {
  const client = new QueryClient()
  onTestFinished(() => client.clear())
  const get = vi
    .spyOn(api, 'get')
    .mockRejectedValue(new Error('network unavailable'))
  const onClose = vi.fn()
  const onPaid = vi.fn()
  render(
    <QueryClientProvider client={client}>
      <AlipayPaymentDialog
        checkout={embeddedCheckout}
        onClose={onClose}
        onPaid={onPaid}
      />
    </QueryClientProvider>
  )
  expect(
    await screen.findByText('Unable to check payment status. Please try again.')
  ).toBeVisible()
  get.mockResolvedValue({
    data: { success: true, data: { status: 'pending' } },
  })
  const user = userEvent.setup()
  await user.click(screen.getByRole('button', { name: 'Check payment status' }))
  expect(await screen.findByText('Waiting for payment')).toBeVisible()
  expect(onPaid).not.toHaveBeenCalled()
  await user.keyboard('{Escape}')
  expect(onClose).toHaveBeenCalledTimes(1)
})

test('stops automatic polling at the waiting deadline and on unmount', async () => {
  vi.useFakeTimers()
  onTestFinished(() => {
    vi.useRealTimers()
  })
  const client = new QueryClient()
  onTestFinished(() => client.clear())
  const get = vi
    .spyOn(api, 'get')
    .mockResolvedValue({ data: { success: true, data: { status: 'pending' } } })
  const { unmount } = render(
    <QueryClientProvider client={client}>
      <AlipayPaymentDialog
        checkout={{
          ...embeddedCheckout,
          expires_at: Math.floor(Date.now() / 1000) + 2,
        }}
        onClose={vi.fn()}
        onPaid={vi.fn()}
      />
    </QueryClientProvider>
  )
  await act(async () => {
    await vi.advanceTimersByTimeAsync(10)
  })
  expect(get).toHaveBeenCalledTimes(1)
  await act(async () => {
    await vi.advanceTimersByTimeAsync(2000)
  })
  expect(
    screen.getByText(
      'Payment waiting period ended. Check your order history before paying again.'
    )
  ).toBeVisible()
  expect(screen.queryByTitle('Alipay payment QR code')).toBeNull()
  await act(async () => {
    await vi.advanceTimersByTimeAsync(3000)
  })
  expect(get).toHaveBeenCalledTimes(1)
  unmount()
  await act(async () => {
    await vi.advanceTimersByTimeAsync(3000)
  })
  expect(get).toHaveBeenCalledTimes(1)
})
