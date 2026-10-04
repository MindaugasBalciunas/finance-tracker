import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { MemoryRouter, Route, Routes, useLocation } from 'react-router-dom'
import Settings from '../pages/Settings'

function Where() {
  return <div data-testid="where">{useLocation().pathname}</div>
}

describe('settings navigation', () => {
  afterEach(() => vi.restoreAllMocks())

  it('tabs navigate to absolute paths and a stacked path recovers', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => new Response('{}', { status: 200 })))
    render(
      <QueryClientProvider client={new QueryClient()}>
        <MemoryRouter initialEntries={['/settings/data/rules/appearance']}>
          <Routes><Route path="/settings/*" element={<><Settings /><Where /></>} /></Routes>
        </MemoryRouter>
      </QueryClientProvider>,
    )
    await waitFor(() => expect(screen.getByTestId('where').textContent).toBe('/settings/appearance'))
    expect(screen.getByRole('button', { name: 'Appearance' })).toHaveClass('text-ink')
    fireEvent.click(screen.getByRole('button', { name: 'Data & backup' }))
    await waitFor(() => expect(screen.getByTestId('where').textContent).toBe('/settings/data'))
    fireEvent.click(screen.getByRole('button', { name: 'Rules' }))
    await waitFor(() => expect(screen.getByTestId('where').textContent).toBe('/settings/rules'))
  })
})
