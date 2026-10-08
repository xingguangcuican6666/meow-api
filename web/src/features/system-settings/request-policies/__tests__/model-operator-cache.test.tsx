import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  createRouter,
  createRootRoute,
  createMemoryHistory,
  RouterContextProvider,
} from '@tanstack/react-router'
import { cleanup, render, screen } from '@testing-library/react'
import { useState } from 'react'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'

import { SettingsPageProvider } from '../../components/settings-page-context'
import { RoutingPolicySection } from '../routing-section'

const operatorModels = ['gpt-4o', 'gpt-4o-mini']
const operatorChannels = [
  { id: 1, type: 1, name: 'Primary operator', status: 1 },
]

let client: QueryClient

function baseOptions(): Record<string, string> {
  return {
    RetryTimes: '2',
    'channel_affinity_setting.enabled': 'false',
    'channel_affinity_setting.session_mode': '',
    'channel_affinity_setting.switch_on_success': 'false',
    'channel_affinity_setting.keep_on_channel_disabled': 'false',
    'channel_affinity_setting.max_entries': '100000',
    'channel_affinity_setting.default_ttl_seconds': '3600',
    'channel_affinity_setting.rules': '[]',
    AutomaticRetryStatusCodes: '429,500-503',
    'model_operator_setting.enabled': 'true',
    // A pinned model is what gives the table a row, and the row's cells are
    // what read the enabled-model list.
    'model_operator_setting.model_channel_map': '{"gpt-4o": 1}',
  }
}

beforeEach(() => {
  // Mirrors createAppQueryClient(): a cached entry is served on first render,
  // so a wrong shape reaches the component before any refetch can correct it.
  client = new QueryClient({
    defaultOptions: {
      queries: {
        retry: false,
        refetchOnWindowFocus: false,
        staleTime: 10 * 1000,
      },
      mutations: { retry: false },
    },
  })
  const options = baseOptions()
  vi.spyOn(api, 'get').mockImplementation(async (url, config) => {
    const params = (config as { params?: { p?: number } } | undefined)?.params
    if (url === '/api/channel/models_enabled') {
      return { data: { success: true, data: [...operatorModels] } }
    }
    if (url === '/api/channel') {
      return {
        data: {
          success: true,
          data: {
            items: params?.p === 1 ? operatorChannels : [],
            total: operatorChannels.length,
            page: params?.p ?? 1,
            page_size: 100,
          },
        },
      }
    }
    return { data: { success: true, data: { options } } }
  })
})

afterEach(() => {
  cleanup()
  client.clear()
  vi.restoreAllMocks()
})

function showRoutingPage() {
  const router = createRouter({
    routeTree: createRootRoute(),
    history: createMemoryHistory({ initialEntries: ['/'] }),
  })
  function Workspace() {
    const [container, setContainer] = useState<HTMLDivElement | null>(null)
    return (
      <>
        <div ref={setContainer} />
        <SettingsPageProvider actionsContainer={container}>
          <RoutingPolicySection />
        </SettingsPageProvider>
      </>
    )
  }
  render(
    <QueryClientProvider client={client}>
      <RouterContextProvider router={router}>
        <Workspace />
      </RouterContextProvider>
    </QueryClientProvider>
  )
}

it('renders the sole operator table after the ratio form cached the enabled models envelope', async () => {
  // model-ratio-form.tsx caches the whole {success,message,data} envelope of
  // getEnabledModels() under ['enabled-models']. Opening request policies after
  // that tab must not feed the envelope to the operator picker, which expects a
  // plain list of model names.
  client.setQueryData(['enabled-models'], {
    success: true,
    message: '',
    data: [...operatorModels],
  })

  showRoutingPage()

  expect(
    await screen.findByRole('switch', { name: 'Enable sole operator routing' })
  ).toBeVisible()
  expect(
    await screen.findByRole('region', { name: 'Sole operator table' })
  ).toBeVisible()
  expect(await screen.findByRole('combobox', { name: 'Model' })).toBeVisible()
})
