import { render, screen, fireEvent } from '@testing-library/react'
import { Delta, Meter, NumberInput, Segmented, Stat } from '../components/ui'
import { OrderTable } from '../components/IBKRPanel'

describe('ui primitives', () => {
  it('Delta shows direction with an arrow and colours by meaning', () => {
    const { container, rerender } = render(<Delta value={500} />)
    expect(container.textContent).toContain('+€500')
    expect(container.firstChild).toHaveClass('text-good')
    rerender(<Delta value={500} goodWhenUp={false} />)
    expect(container.firstChild).toHaveClass('text-bad')
    rerender(<Delta value={0.2} />)
    expect(container.textContent).toBe('±0')
  })

  it('Meter caps the fill, flags overspend and celebrates a reached goal', () => {
    const { container, rerender } = render(<Meter value={150} max={100} />)
    const bar = () => container.querySelector('[role=meter] > div') as HTMLElement
    expect(bar().style.width).toBe('100%')
    expect(bar()).toHaveClass('bg-bad')
    rerender(<Meter value={95} max={100} />)
    expect(bar()).toHaveClass('bg-warn')
    rerender(<Meter value={1000} max={1000} goal />)
    expect(bar()).toHaveClass('bg-good')
  })

  it('Segmented reports the picked value', () => {
    const picked: string[] = []
    render(<Segmented value="a" onChange={(v) => picked.push(v)} options={[{ value: 'a', label: 'A' }, { value: 'b', label: 'B' }]} />)
    fireEvent.click(screen.getByText('B'))
    expect(picked).toEqual(['b'])
  })

  it('Stat renders label, value and context', () => {
    render(<Stat label="Saved" value="€3,009" sub="97% of income" tone="good" />)
    expect(screen.getByText('€3,009')).toHaveClass('text-good')
    expect(screen.getByText('97% of income')).toBeInTheDocument()
  })
})

describe('NumberInput', () => {
  it('keeps what is typed, accepts commas and reports numbers only', () => {
    const got: (number | undefined)[] = []
    render(<NumberInput value={undefined} onChange={(v) => got.push(v)} aria-label="shares" />)
    const el = screen.getByLabelText('shares') as HTMLInputElement
    for (const t of ['0', '0,', '0,5', '0,5x']) fireEvent.change(el, { target: { value: t } })
    expect(el.value).toBe('0,5x')                 // never rewritten to NaN
    expect(got).toEqual([0, 0, 0.5, undefined])
    expect(el).toHaveAttribute('aria-invalid', 'true')
    fireEvent.change(el, { target: { value: '12.25' } })
    expect(el).not.toHaveAttribute('aria-invalid')
    expect(got[got.length - 1]).toBe(12.25)
  })
})

describe('IBKR open orders', () => {
  it('lists each order as a table row: side, symbol, shares, limit, time in force, status', () => {
    // The order IBKR returned on 6 Nov, as the server now reads it.
    render(<OrderTable orders={[{ id: '15555562', symbol: 'VWCE', side: 'buy', type: 'limit', status: 'REPLACED', quantity: 6, filled: 0, remaining: 6, price: 162.8, tif: 'GTC', created: '2026-11-06T06:24:15Z' }]} />)
    expect(screen.getAllByRole('columnheader').map((h) => h.textContent)).toEqual(['Order', 'Shares', 'Filled · left', 'Type', 'Limit', 'Valid', 'Status', 'Placed'])
    const row = screen.getAllByRole('row')[1]
    expect(row).toHaveTextContent('buyVWCE')
    expect(row).toHaveTextContent('162.80')
    expect(row).toHaveTextContent('GTC')
    expect(row).toHaveTextContent('Replaced') // in its column, and under the symbol on a phone
    expect(row).toHaveTextContent('0 · 6')
    expect(screen.getByText('buy')).toHaveClass('text-good')
  })
  it('reads IBKR status spellings as words', () => {
    render(<OrderTable orders={[{ id: '1', symbol: 'VALL', side: 'sell', type: 'LIMIT', status: 'PreSubmitted', quantity: 10, price: 4.2 }, { id: '2', symbol: 'BRK B', side: 'buy', type: 'market', status: 'PENDING_CANCEL', quantity: 1 }]} />)
    expect(screen.getAllByText('Pre submitted').length).toBeGreaterThan(0)
    expect(screen.getAllByText('Pending cancel').length).toBeGreaterThan(0)
    expect(screen.getByText('BRK B')).toBeInTheDocument()
  })
})

describe('a finished month in the plan', () => {
  it('shows the income that came in and where it went, without the principal twice', async () => {
    const { IncomeUsed } = await import('../pages/Plan')
    // October 2026: fixed €2,361.66 of which €438.95 is mortgage principal, also in saved.
    const r = { month: '2026-10', income_actual: 7884.92, fixed_spent: 2361.66, fixed_expense: 1922.71, saved_actual: 508.44, discretionary_spent: 2077.87 } as any
    render(<IncomeUsed r={r} />)
    expect(document.body).toHaveTextContent('Of the €7,885 that came in, you spent €4,001 (51%) and had €3,376 left over.')
    expect(screen.getByText('Obligations').nextSibling).toHaveTextContent('€1,923')
    expect(screen.getByRole('img').getAttribute('aria-label')).toContain('left over €3,376')
  })
})
