/*
Copyright (C) 2023-2026 xingguangcuican6666

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
*/

import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  createMemoryHistory,
  createRootRoute,
  createRouter,
  RouterProvider,
} from '@tanstack/react-router'
import { cleanup, render, screen } from '@testing-library/react'
import i18next from 'i18next'
import { I18nextProvider } from 'react-i18next'
import { afterEach, describe, expect, it } from 'vitest'

import { AccountSuspendedError } from '../account-suspended'

const i18n = i18next.createInstance()
void i18n.init({
  lng: 'en',
  resources: { en: { translation: {} } },
  fallbackLng: 'en',
  nsSeparator: false,
  interpolation: { escapeValue: false },
})

const client = new QueryClient({
  defaultOptions: { queries: { retry: false } },
})

afterEach(() => {
  cleanup()
  client.clear()
})

async function renderPage() {
  const router = createRouter({
    routeTree: createRootRoute({ component: AccountSuspendedError }),
    history: createMemoryHistory({ initialEntries: ['/account-suspended'] }),
  })
  await router.load()
  return render(
    <QueryClientProvider client={client}>
      <I18nextProvider i18n={i18n}>
        <RouterProvider router={router} />
      </I18nextProvider>
    </QueryClientProvider>
  )
}

describe('account-suspended page', () => {
  it('states the ban with the one message the backend also returns', async () => {
    await renderPage()

    const heading = await screen.findByRole('heading', { level: 1 })
    expect(heading).toHaveTextContent('Your account has been suspend!')
  })

  it('is a page landmark so assistive technology can jump to the verdict', async () => {
    const view = await renderPage()
    expect(view.container.querySelector('main')).not.toBeNull()
  })

  it('offers a way to another account and a way back home', async () => {
    await renderPage()

    const anotherAccount = screen.getByRole('button', {
      name: 'Sign in with another account',
    })
    expect(anotherAccount).toHaveAttribute('href', '/sign-in')

    const home = screen.getByRole('button', { name: 'Back to Home' })
    expect(home).toHaveAttribute('href', '/')
  })

  it('explains that the existing sessions and API keys were revoked', async () => {
    await renderPage()

    expect(
      screen.getByText(
        'Your existing sessions and API keys have been revoked, and this account can no longer place requests.'
      )
    ).toBeVisible()
  })

  it('points to the administrator when the ban looks like a mistake', async () => {
    await renderPage()

    expect(screen.getByText(/administrator of this site/)).toBeVisible()
  })
})
