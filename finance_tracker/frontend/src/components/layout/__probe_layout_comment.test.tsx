// TEMPORARY VERIFICATION PROBE — delete after run. Never touches finance.db or the server.
import { describe, it, expect } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import { MemoryRouter, Routes, Route } from 'react-router-dom'
import Layout from './Layout'
import More from '../../pages/More'
import { DateRangeProvider } from '../../context/DateRangeContext'

function renderApp(initialPath = '/') {
  return render(
    <DateRangeProvider>
      <MemoryRouter initialEntries={[initialPath]}>
        <Routes>
          <Route element={<Layout />}>
            <Route path="/" element={<div>home page</div>} />
            <Route path="/more" element={<More />} />
            <Route path="/stocks" element={<div>stocks page</div>} />
          </Route>
        </Routes>
      </MemoryRouter>
    </DateRangeProvider>
  )
}

describe('probe: what the mobile header hamburger actually does', () => {
  it('hamburger exists in top bar but opens ONLY the date-range panel (no nav links)', () => {
    renderApp('/')
    const burger = screen.getByRole('button', { name: 'Date range menu' })
    expect(burger).toBeInTheDocument()
    // Panel closed initially
    expect(screen.queryByText('Date range')).not.toBeInTheDocument()
    fireEvent.click(burger)
    // Panel now open: contains date presets, not navigation
    expect(screen.getByText('Date range')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'This mo' })).toBeInTheDocument()
    // No page-navigation links inside the toggled panel: every nav link on the
    // page belongs to the desktop header nav or the bottom bar, both rendered
    // unconditionally. Count links before/after toggle to prove the hamburger
    // adds zero navigation entries.
    fireEvent.click(burger) // close
    const linksClosed = screen.getAllByRole('link').length
    fireEvent.click(burger) // open
    const linksOpen = screen.getAllByRole('link').length
    expect(linksOpen).toBe(linksClosed)
  })

  it('bottom tab bar has a Stocks tab; More page no longer links Stocks', () => {
    renderApp('/more')
    // Stocks now lives in the bottom bar (aria-label on the NavLink)
    const stocksTabs = screen.getAllByRole('link', { name: 'Stocks' })
    expect(stocksTabs.some((a) => a.getAttribute('href') === '/stocks')).toBe(true)
    // The More page content renders, and contains no Stocks entry of its own:
    // the only /stocks link in the whole document is the bottom-bar tab.
    expect(screen.getByText("Everything that doesn't need a tab of its own")).toBeInTheDocument()
    const allStocksLinks = screen.getAllByRole('link').filter((a) => a.getAttribute('href') === '/stocks')
    expect(allStocksLinks.length).toBe(1)
    expect(allStocksLinks[0].getAttribute('aria-label')).toBe('Stocks')
  })

  it('date menu closes on navigation (no lingering overlay)', () => {
    renderApp('/')
    fireEvent.click(screen.getByRole('button', { name: 'Date range menu' }))
    expect(screen.getByText('Date range')).toBeInTheDocument()
    fireEvent.click(screen.getAllByRole('link', { name: 'More' })[0])
    expect(screen.queryByText('Date range')).not.toBeInTheDocument()
  })
})
