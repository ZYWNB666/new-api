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
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'

import type { User } from '../../types'
import { UsersMutateDrawer } from '../users-mutate-drawer'
import { UsersProvider } from '../users-provider'

const target: User = {
  id: 2,
  username: 'managed-admin',
  display_name: 'Managed admin',
  role: 10,
  status: 1,
  quota: 0,
  used_quota: 0,
  request_count: 0,
  group: 'default',
}
const label = "View other accounts' audit logs"
const description =
  'View audit records from user and admin roles. Root records are always excluded.'

function renderPermissions(
  viewerRole: number,
  allowed?: boolean,
  adaptiveRouting = false,
  clientRequestIDHeader = ''
) {
  useAuthStore
    .getState()
    .auth.setUser({ id: 1, username: 'operator', role: viewerRole })
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (url === '/api/authz/catalog') {
      return {
        data: {
          success: true,
          data: {
            resources: [
              {
                resource: 'audit',
                label_key: 'Audit Logs',
                actions: [
                  {
                    action: 'read',
                    label_key: label,
                    description_key: description,
                  },
                ],
              },
            ],
            roles: [{ key: 'admin', grants: { audit: { read: false } } }],
          },
        },
      }
    }
    if (url === '/api/group/') {
      return { data: { success: true, data: ['default'] } }
    }
    return {
      data: {
        success: true,
        data: {
          ...target,
          adaptive_routing: adaptiveRouting,
          client_request_id_header: clientRequestIDHeader,
          admin_permissions:
            allowed === undefined ? {} : { audit: { read: allowed } },
        },
      },
    }
  })
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={client}>
      <UsersProvider>
        <UsersMutateDrawer
          open
          onOpenChange={() => undefined}
          currentRow={target}
        />
      </UsersProvider>
    </QueryClientProvider>
  )
}

afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
  useAuthStore.getState().auth.reset()
})

it.each([undefined, true])(
  'root can save an audit grant or revocation from the existing editor (previous=%s)',
  async (allowed) => {
    const put = vi
      .spyOn(api, 'put')
      .mockResolvedValue({ data: { success: true } })
    renderPermissions(100, allowed)
    await screen.findByDisplayValue('Managed admin')
    const checkbox = await screen.findByRole('checkbox', {
      name: new RegExp(label),
    })
    await waitFor(() =>
      expect(checkbox).toHaveAttribute('aria-checked', String(!!allowed))
    )
    expect(screen.getByText(description)).toBeVisible()
    await userEvent.click(checkbox)
    await userEvent.click(screen.getByRole('button', { name: 'Save changes' }))
    await waitFor(() =>
      expect(put).toHaveBeenCalledWith(
        '/api/user/',
        expect.objectContaining({
          id: 2,
          admin_permissions: { audit: { read: !allowed } },
        })
      )
    )
  }
)

it('admin cannot edit the audit permission even when the catalog is available', async () => {
  renderPermissions(10)
  await screen.findByDisplayValue('Managed admin')
  expect(
    screen.queryByRole('checkbox', { name: new RegExp(label) })
  ).not.toBeInTheDocument()
})

it.each([false, true])(
  'administrator can toggle adaptive routing and save it (previous=%s)',
  async (enabled) => {
    const put = vi
      .spyOn(api, 'put')
      .mockResolvedValue({ data: { success: true } })
    renderPermissions(100, undefined, enabled)
    await screen.findByDisplayValue('Managed admin')
    const toggle = await screen.findByRole('switch', {
      name: 'Adaptive routing',
    })
    await waitFor(() =>
      expect(toggle).toHaveAttribute('aria-checked', String(enabled))
    )
    toggle.focus()
    await userEvent.keyboard(' ')
    expect(toggle).toHaveAttribute('aria-checked', String(!enabled))
    await userEvent.click(screen.getByRole('button', { name: 'Save changes' }))
    await waitFor(() =>
      expect(put).toHaveBeenCalledWith(
        '/api/user/',
        expect.objectContaining({ id: 2, adaptive_routing: !enabled })
      )
    )
  }
)

it.each(['X-Client-Request-Id', 're:^X-(Client|Customer)-Request-Id$', ''])(
  'administrator can load and save client request ID header rule %s',
  async (rule) => {
    const put = vi
      .spyOn(api, 'put')
      .mockResolvedValue({ data: { success: true } })
    renderPermissions(100, undefined, false, 'X-Old-Request-Id')
    await screen.findByDisplayValue('Managed admin')
    const input = screen.getByLabelText('Client request ID header')
    expect(input).toHaveValue('X-Old-Request-Id')
    await userEvent.clear(input)
    // Paste regex syntax without interpreting brackets as keyboard commands.
    fireEvent.change(input, { target: { value: rule } })
    await userEvent.click(screen.getByRole('button', { name: 'Save changes' }))
    await waitFor(() =>
      expect(put).toHaveBeenCalledWith(
        '/api/user/',
        expect.objectContaining({ id: 2, client_request_id_header: rule })
      )
    )
  }
)

it('rejects oversized client header rules before saving', async () => {
  const put = vi.spyOn(api, 'put')
  renderPermissions(100)
  await screen.findByDisplayValue('Managed admin')
  fireEvent.change(screen.getByLabelText('Client request ID header'), {
    target: { value: 'x'.repeat(257) },
  })
  await userEvent.click(screen.getByRole('button', { name: 'Save changes' }))
  expect(
    await screen.findByText('Header rule must not exceed 256 characters')
  ).toBeVisible()
  expect(put).not.toHaveBeenCalled()
})
