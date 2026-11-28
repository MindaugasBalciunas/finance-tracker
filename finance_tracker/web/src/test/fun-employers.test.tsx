import { render, screen } from '@testing-library/react'
import { HashRouter } from 'react-router-dom'
import { EmployerMonths, type Fact } from '../pages/insights/Fun'

// In a single year a year block read as a full year's pay: Nexos.ai had paid
// four times. A short period gets one dot per month paid instead.
describe('who paid you the most, over a short period', () => {
  it('draws a dot for each month an employer paid, on a month axis', () => {
    const f: Fact = { key: 'employers', icon: 'briefcase', title: 'Who paid you the most', value: 'Nexos.ai', items: [
      { label: 'Nexos.ai', value: '€11,662', months: ['2026-08', '2026-09', '2026-10', '2026-11'] },
      { label: 'Vialet', value: '€4,125', months: ['2026-01', '2026-03'] },
    ] }
    const { container } = render(<HashRouter><EmployerMonths f={f} color="var(--s1)" /></HashRouter>)
    const dots = (who: string) => [...container.querySelectorAll('[data-month]')].filter((d) => d.getAttribute('title')?.startsWith(who)).map((d) => d.getAttribute('data-month'))
    expect(dots('Nexos.ai')).toEqual(['2026-08', '2026-09', '2026-10', '2026-11'])
    expect(dots('Vialet')).toEqual(['2026-01', '2026-03']) // February stays a gap
    expect(screen.getByText('Aug–Nov ’26')).toBeInTheDocument()
    expect(screen.getByText(/Each dot is a month/)).toBeInTheDocument()
  })
})
