import { render, screen } from '@testing-library/react'
import { Checks, SpendCalendar } from '../pages/insights/MonthReview'

describe('month review pieces', () => {
  it('Checks links each finding and flags warnings', () => {
    const { container, rerender } = render(<Checks checks={[
      { level: 'warn', text: 'Swedbank balance is 40 days old', link: '/wealth' },
      { level: 'info', text: '3 transactions in the inbox', link: '/ledger/inbox' },
    ]} />)
    expect(screen.getByText('Worth a look')).toBeInTheDocument()
    const links = container.querySelectorAll('a')
    expect(links[0].getAttribute('href')).toBe('#/wealth')
    expect(links[1].getAttribute('href')).toBe('#/ledger/inbox')
    expect(links[0].querySelector('svg')).toHaveClass('text-warn')
    expect(links[1].querySelector('svg')).toHaveClass('text-muted')
    rerender(<Checks checks={[]} />)
    expect(screen.queryByText('Worth a look')).toBeNull()
  })

  it('SpendCalendar starts on Monday and shades by spend', () => {
    // 1 Sept 2026 is a Tuesday: one blank lead cell.
    const days = Array.from({ length: 30 }, (_, i) => ({ date: `2026-09-${String(i + 1).padStart(2, '0')}`, spent: i === 4 ? 200 : 0 }))
    const { container } = render(<SpendCalendar days={days} />)
    const cells = container.querySelectorAll('.grid-cols-7')[1].children
    expect(cells.length).toBe(31)
    expect(cells[0].textContent).toBe('')
    expect(cells[1].getAttribute('title')).toContain('2026-09-01')
    const busy = cells[5] as HTMLElement
    expect(busy.getAttribute('title')).toBe('2026-09-05: €200.00')
    expect(busy.style.color).toBe('white')
    expect((cells[2] as HTMLElement).style.background).toContain('--sunken')
  })
})
