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
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { expect, it, onTestFinished, vi } from 'vitest'

import { SettingsPageProvider } from '../../components/settings-page-context'
import { PaymentSettingsSection } from '../payment-settings-section'

const update = vi.hoisted(() => vi.fn())
vi.mock('../../hooks/use-update-option', () => ({
  useUpdateOption: () => ({ mutateAsync: update, isPending: false }),
}))

it('saves Alipay seller and PEM keys and allows clearing the callback overrides', async () => {
  update.mockResolvedValue({ success: true })
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  const defaults = {
    PayAddress: '',
    EpayId: '',
    EpayKey: '',
    Price: 7.3,
    MinTopUp: 1,
    CustomCallbackAddress: '',
    PayMethods: '[]',
    AmountOptions: '[]',
    AmountDiscount: '{}',
    StripeApiSecret: '',
    StripeWebhookSecret: '',
    StripePriceId: '',
    StripeUnitPrice: 1,
    StripeMinTopUp: 1,
    StripePromotionCodesEnabled: false,
    CreemApiKey: '',
    CreemWebhookSecret: '',
    CreemTestMode: false,
    CreemProducts: '[]',
    AlipayAppId: '',
    AlipaySellerId: '',
    AlipayAppPrivateKey: '',
    AlipayPublicKey: '',
    AlipayUnitPrice: 7.3,
    AlipayMinTopUp: 1,
    AlipayNotifyUrl: 'https://old.example.com/notify',
    AlipayReturnUrl: 'https://old.example.com/wallet',
    AlipaySandbox: false,
  }
  const waffo = {
    WaffoEnabled: false,
    WaffoApiKey: '',
    WaffoPrivateKey: '',
    WaffoPublicCert: '',
    WaffoSandboxPublicCert: '',
    WaffoSandboxApiKey: '',
    WaffoSandboxPrivateKey: '',
    WaffoSandbox: false,
    WaffoMerchantId: '',
    WaffoCurrency: 'USD',
    WaffoUnitPrice: 1,
    WaffoMinTopUp: 1,
    WaffoNotifyUrl: '',
    WaffoReturnUrl: '',
    WaffoPayMethods: '[]',
  }
  const actions = document.createElement('div')
  document.body.append(actions)
  onTestFinished(() => {
    actions.remove()
    queryClient.clear()
  })
  render(
    <QueryClientProvider client={queryClient}>
      <SettingsPageProvider actionsContainer={actions}>
        <PaymentSettingsSection
          defaultValues={defaults}
          waffoDefaultValues={waffo}
          waffoPancakeDefaultValues={{
            WaffoPancakeMerchantID: '',
            WaffoPancakePrivateKey: '',
            WaffoPancakeReturnURL: '',
          }}
          complianceDefaults={{
            confirmed: true,
            termsVersion: 'v1',
            confirmedAt: 1,
            confirmedBy: 1,
          }}
        />
      </SettingsPageProvider>
    </QueryClientProvider>
  )
  const user = userEvent.setup()
  await user.click(screen.getByRole('tab', { name: 'Alipay' }))
  const key = screen.getByLabelText('Application private key')
  expect(key.tagName).toBe('TEXTAREA')
  const publicKey = screen.getByLabelText('Alipay public key')
  expect(publicKey.tagName).toBe('TEXTAREA')
  fireEvent.change(key, {
    target: {
      value: '-----BEGIN PRIVATE KEY-----\nexample\n-----END PRIVATE KEY-----',
    },
  })
  fireEvent.change(publicKey, { target: { value: 'example-public-key' } })
  fireEvent.change(screen.getByLabelText('Seller ID (PID)'), {
    target: { value: '2088000000000000' },
  })
  await user.clear(screen.getByLabelText('Notify URL'))
  await user.clear(screen.getByLabelText('Return URL'))
  await user.click(screen.getByRole('button', { name: 'Save all settings' }))
  await waitFor(() => {
    expect(update).toHaveBeenCalledWith({
      key: 'AlipaySellerId',
      value: '2088000000000000',
    })
    expect(update).toHaveBeenCalledWith({ key: 'AlipayNotifyUrl', value: '' })
    expect(update).toHaveBeenCalledWith({ key: 'AlipayReturnUrl', value: '' })
    expect(update).toHaveBeenCalledWith({
      key: 'AlipayAppPrivateKey',
      value: '-----BEGIN PRIVATE KEY-----\nexample\n-----END PRIVATE KEY-----',
    })
  })
  queryClient.clear()
})
