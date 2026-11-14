import { render, screen } from '@testing-library/react'
import { HashRouter } from 'react-router-dom'
import { FreeBudgetCard } from '../pages/home/FreeBudget'
import type { FreeBudget } from '../lib/types'

const part = (budgeted: number, spent: number, lines: string[]) => ({ budgeted, spent, left: budgeted - spent, lines })
const free = (needs = part(700, 300, ['Food', 'Kids']), wants = part(300, 120, ['Leisure & shopping'])): FreeBudget => ({
  budgeted: needs.budgeted + wants.budgeted, spent: needs.spent + wants.spent, left: needs.left + wants.left,
  lines: [...needs.lines, ...wants.lines], needs, wants,
})
const show = (f: FreeBudget, unreviewed?: number) => render(<HashRouter><FreeBudgetCard f={f} month="November 2026" unreviewed={unreviewed} /></HashRouter>)
// "€400 left": the figure and its word sit in separate elements.
const tile = (amount: string) => screen.getByText((_, el) => el?.tagName === 'DIV' && el.textContent === `${amount} left`)

describe('Home spending budget', () => {
  it('shows what is left of the budget, and needs and wants side by side', () => {
    show(free())
    expect(screen.getByText('Spending budget · November 2026')).toBeInTheDocument()
    expect(screen.getByText('€580')).toHaveClass('text-good')
    expect(screen.getByText('€1,000')).toBeInTheDocument()
    expect(tile('€400')).toBeInTheDocument() // needs
    expect(tile('€180')).toBeInTheDocument() // wants
    expect(screen.getByText(/wants are 30% of the budget/)).toBeInTheDocument()
    expect(screen.getByText('Wants: Leisure & shopping')).toBeInTheDocument()
    expect(screen.getByRole('img', { name: 'Wants: €120 of €300 spent' })).toBeInTheDocument()
    expect(screen.getByRole('link')).toHaveAttribute('href', '#/plan')
  })

  it('says when wants have gone over, in red, while the budget as a whole still has room', () => {
    show(free(part(700, 300, ['Food']), part(300, 450, ['Dating'])))
    const over = screen.getByText((_, el) => el?.tagName === 'DIV' && el.textContent === '€150 over')
    expect(over).toHaveClass('text-bad')
    expect(screen.getByText('€250')).toHaveClass('text-good')
  })

  it('splits what is left into day-to-day and kept in funds, as Left to spend sees it', () => {
    // Prod, November 2026: €1,433.54 left, €791.33 of it held by funds.
    show({ ...free(part(1300, 544.59, ['Food']), part(1073.46, 395.33, ['Travel'])), in_funds: 791.33, fund_spent: 302.13 })
    expect(screen.getByText('Day-to-day').nextSibling).toHaveTextContent('€642.21')
    expect(screen.getByText('Kept in funds').nextSibling).toHaveTextContent('€791.33')
    expect(screen.getByText(/from funds/).parentElement).toHaveTextContent('€939.92 spent (€302.13 from funds)')
  })

  it('marks day-to-day over in red while funds still hold money', () => {
    show({ ...free(part(400, 700, ['Food']), part(1000, 0, ['Gifts'])), in_funds: 1000, fund_spent: 0 })
    expect(screen.getByText('Day-to-day').nextSibling).toHaveTextContent('€300 over')
    expect(screen.getByText('Day-to-day').nextSibling).toHaveClass('text-bad')
  })

  it('leaves the split out without fund money, or when the budget is over', () => {
    show({ ...free(), in_funds: 0 })
    expect(screen.queryByText('Kept in funds')).toBeNull()
    show({ ...free(part(700, 900, ['Food']), part(300, 450, ['Travel'])), in_funds: 50 })
    expect(screen.queryByText('Kept in funds')).toBeNull()
  })

  it('reads plainly when the whole budget is over', () => {
    show(free(part(700, 900, ['Food']), part(300, 450, ['Dating'])))
    expect(screen.getByText('€350')).toHaveClass('text-bad')
    expect(screen.getByText((_, el) => el?.tagName === 'SPAN' && el.textContent === 'over the €1,000 budget')).toBeInTheDocument()
  })

  it('names inbox spending that is not counted yet', () => {
    show(free(), 42)
    expect(screen.getByText(/in the bank inbox, not in these lines yet/)).toBeInTheDocument()
    expect(screen.getByText('€42')).toBeInTheDocument()
  })

  it('stays hidden without spending lines', () => {
    const { container } = show(free(part(0, 0, []), part(0, 0, [])))
    expect(container).toBeEmptyDOMElement()
  })
})

describe('Plan spending budget', () => {
  it('shows the same card without a link back to Plan', () => {
    render(<HashRouter><FreeBudgetCard f={free()} month="September 2026" link={false} /></HashRouter>)
    expect(screen.getByText('Spending budget · September 2026')).toBeInTheDocument()
    expect(screen.queryByRole('link')).toBeNull()
  })
})
