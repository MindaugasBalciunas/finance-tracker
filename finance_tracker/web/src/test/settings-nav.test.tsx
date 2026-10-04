import { render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { MemoryRouter, Route, Routes, useLocation } from 'react-router-dom'
import Settings from '../pages/Settings'

function Where() {
  return <div data-testid="where">{useLocation().pathname}</div>
}

describe('settings navigation', () => {
  afterEach(() => vi.restoreAllMocks())

  it('section links are absolute and a stacked path recovers', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => new Response('{}', { status: 200 })))
    render(
      <QueryClientProvider client={new QueryClient()}>
        <MemoryRouter initialEntries={['/settings/data/rules/appearance']}>
          <Routes><Route path="/settings/*" element={<><Settings /><Where /></>} /></Routes>
        </MemoryRouter>
      </QueryClientProvider>,
    )
    await waitFor(() => expect(screen.getByTestId('where').textContent).toBe('/settings/appearance'))
    expect(screen.getByText('Data & backup').closest('a')!.getAttribute('href')).toBe('/settings/data')
    expect(screen.getByText('Appearance').closest('a')).toHaveClass('chip-on')
  })
})
