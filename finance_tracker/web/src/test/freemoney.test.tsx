import { render, screen } from '@testing-library/react'
import { HashRouter } from 'react-router-dom'
import { FreeMoneyCard } from '../pages/home/FreeMoney'
import type { FreeMoney } from '../lib/types'

// Prod, November 2026 (the essential/optional split is made up here).
const nov: FreeMoney = { income: 6050, fixed: 2361.66, saving: 1200, free: 2488.34, spent: 939.92,
  essential: 600, optional: 339.92, unreviewed: 0, in_funds: 737.87, left_to_spend: 810.55 }
const show = (f: FreeMoney, link = true) => render(<HashRouter><FreeMoneyCard f={f} month="November 2026" link={link} /></HashRouter>)
const row = (label: string) => screen.getByText(label).closest('div')!

describe('Free money card', () => {
  it('reads from salary to left to spend, with spending split into essential and optional', () => {
    show(nov)
    expect(screen.getByText('Free money · November 2026')).toBeInTheDocument()
    expect(screen.getByText('€2,488')).toBeInTheDocument()
    expect(screen.getByText('€6,050 income − €2,362 fixed − €1,200 saving')).toBeInTheDocument()
    expect(screen.getByText('from income after fixed costs and saving')).toBeInTheDocument()
    expect(row('Essential')).toHaveTextContent('Essential64%€600')
    expect(row('Optional')).toHaveTextContent('Optional36%€339.92')
    expect(row('Spent')).toHaveTextContent('€939.92')
    expect(row('Kept in funds')).toHaveTextContent('€737.87')
    expect(row('Left to spend')).toHaveTextContent('€810.55')
    expect(row('Left to spend')).toHaveClass('text-good')
    expect(screen.queryByText('Not reviewed yet')).toBeNull()
    expect(screen.getByRole('link')).toHaveAttribute('href', '#/plan')
  })

  it('shows inbox spending apart, and an overspent month in red', () => {
    show({ ...nov, spent: 3000, essential: 1800, optional: 1100, unreviewed: 100, in_funds: 0, left_to_spend: -511.66 })
    expect(row('Not reviewed yet')).toHaveTextContent('€100')
    expect(screen.queryByText('Kept in funds')).toBeNull()
    expect(row('Over')).toHaveTextContent('€511.66')
    expect(row('Over')).toHaveClass('text-bad')
  })

  it('shows refunds and money funds saved in earlier months, so the rows still add up', () => {
    // €3,000 free; a €1,200 flight from a Travel fund that held €1,100 before; €20 back.
    show({ income: 3000, fixed: 0, saving: 0, free: 3000, spent: 1180, essential: 0, optional: 1200, refunds: 20, unreviewed: 0, in_funds: -1100, left_to_spend: 2920 })
    expect(row('Refunds')).toHaveTextContent('−€20')
    expect(row('From funds saved earlier')).toHaveTextContent('+€1,100')
    expect(screen.queryByText('Kept in funds')).toBeNull()
    expect(row('Left to spend')).toHaveTextContent('€2,920')
    expect(screen.getByRole('img')).toHaveAccessibleName(/€1,100 from funds saved earlier; left to spend €2,920/)
  })

  it('has no link on Plan, and stays hidden for an empty month', () => {
    const { container } = show({ ...nov, income: 0, spent: 0 })
    expect(container).toBeEmptyDOMElement()
    show(nov, false)
    expect(screen.queryByRole('link')).toBeNull()
  })
})
