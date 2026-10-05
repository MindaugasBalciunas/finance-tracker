import { render, screen, fireEvent } from '@testing-library/react'
import { Delta, Meter, NumberInput, Segmented, Stat } from '../components/ui'

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
