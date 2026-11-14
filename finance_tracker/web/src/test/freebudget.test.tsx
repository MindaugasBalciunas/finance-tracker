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

describe('Home free budget', () => {
  it('shows what is left of the free budget and how much of it is wants', () => {
    show(free())
    expect(screen.getByText('Free budget in November 2026')).toBeInTheDocument()
    expect(screen.getByText('€580')).toHaveClass('text-good')
    expect(screen.getByText('€1,000')).toBeInTheDocument()
    expect(screen.getByText('€400 left')).toBeInTheDocument() // needs
    expect(screen.getByText('€180 left')).toBeInTheDocument() // wants
    expect(screen.getByText(/· 30% of the budget/)).toBeInTheDocument()
    expect(screen.getByText(/Wants: Leisure & shopping\./)).toBeInTheDocument()
    expect(screen.getByRole('link')).toHaveAttribute('href', '#/plan')
  })

  it('says when wants have gone over, in red', () => {
    show(free(part(700, 300, ['Food']), part(300, 450, ['Dating'])))
    expect(screen.getByText('€150 over')).toHaveClass('text-bad')
    expect(screen.getByText('€250')).toHaveClass('text-good') // the budget as a whole still has room
  })

  it('names inbox spending that is not counted yet', () => {
    show(free(), 42)
    expect(screen.getByText(/€42 in the inbox is not counted yet/)).toBeInTheDocument()
  })

  it('stays hidden without spending lines', () => {
    const { container } = show(free(part(0, 0, []), part(0, 0, [])))
    expect(container).toBeEmptyDOMElement()
  })
})
