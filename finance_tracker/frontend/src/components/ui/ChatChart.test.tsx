import { render } from '@testing-library/react'
import { describe, it, expect, beforeAll } from 'vitest'
import ChatChart, { parseChartSpec } from './ChatChart'

// Recharts' ResponsiveContainer relies on ResizeObserver, which jsdom lacks.
beforeAll(() => {
  if (typeof globalThis.ResizeObserver === 'undefined') {
    globalThis.ResizeObserver = class {
      observe() {}
      unobserve() {}
      disconnect() {}
    } as unknown as typeof ResizeObserver
  }
})

describe('parseChartSpec', () => {
  it('parses a valid bar spec', () => {
    const spec = parseChartSpec(
      JSON.stringify({
        type: 'bar',
        title: 'Spending',
        x: 'month',
        unit: '€',
        series: [{ key: 'amount', name: 'Amount' }],
        data: [{ month: 'Jan', amount: 100 }, { month: 'Feb', amount: 200 }],
      })
    )
    expect(spec).not.toBeNull()
    expect(spec?.type).toBe('bar')
    expect(spec?.x).toBe('month')
    expect(spec?.series).toHaveLength(1)
    expect(spec?.series[0].key).toBe('amount')
    expect(spec?.data).toHaveLength(2)
  })

  it('parses a valid line spec with multiple series', () => {
    const spec = parseChartSpec(
      JSON.stringify({
        type: 'line',
        x: 'day',
        series: [{ key: 'in' }, { key: 'out', color: '#ff0000' }],
        data: [{ day: 'Mon', in: 5, out: 3 }],
      })
    )
    expect(spec?.type).toBe('line')
    expect(spec?.series).toHaveLength(2)
    expect(spec?.series[1].color).toBe('#ff0000')
  })

  it('parses a valid pie spec', () => {
    const spec = parseChartSpec(
      JSON.stringify({
        type: 'pie',
        x: 'category',
        series: [{ key: 'value' }],
        data: [{ category: 'Food', value: 40 }, { category: 'Rent', value: 60 }],
      })
    )
    expect(spec?.type).toBe('pie')
    expect(spec?.series[0].key).toBe('value')
  })

  it('drops series entries without a key but keeps valid ones', () => {
    const spec = parseChartSpec(
      JSON.stringify({
        type: 'area',
        x: 'month',
        series: [{ name: 'no key' }, { key: 'good' }],
        data: [{ month: 'Jan', good: 1 }],
      })
    )
    expect(spec?.series).toHaveLength(1)
    expect(spec?.series[0].key).toBe('good')
  })

  it('returns null for an unknown type', () => {
    expect(
      parseChartSpec(
        JSON.stringify({ type: 'scatter', x: 'x', series: [{ key: 'y' }], data: [{ x: 1, y: 2 }] })
      )
    ).toBeNull()
  })

  it('returns null for empty data', () => {
    expect(
      parseChartSpec(JSON.stringify({ type: 'bar', x: 'x', series: [{ key: 'y' }], data: [] }))
    ).toBeNull()
  })

  it('returns null for non-array data', () => {
    expect(
      parseChartSpec(JSON.stringify({ type: 'bar', x: 'x', series: [{ key: 'y' }], data: {} }))
    ).toBeNull()
  })

  it('returns null when no series has a key', () => {
    expect(
      parseChartSpec(
        JSON.stringify({ type: 'bar', x: 'x', series: [{ name: 'no key' }], data: [{ x: 1 }] })
      )
    ).toBeNull()
  })

  it('returns null when series is missing', () => {
    expect(parseChartSpec(JSON.stringify({ type: 'bar', x: 'x', data: [{ x: 1 }] }))).toBeNull()
  })

  it('returns null for a missing or empty x', () => {
    expect(
      parseChartSpec(JSON.stringify({ type: 'bar', series: [{ key: 'y' }], data: [{ y: 1 }] }))
    ).toBeNull()
    expect(
      parseChartSpec(
        JSON.stringify({ type: 'bar', x: '   ', series: [{ key: 'y' }], data: [{ y: 1 }] })
      )
    ).toBeNull()
  })

  it('returns null for malformed JSON', () => {
    expect(parseChartSpec('{ not json')).toBeNull()
    expect(parseChartSpec('42')).toBeNull()
    expect(parseChartSpec('null')).toBeNull()
  })
})

describe('ChatChart render', () => {
  // jsdom gives ResponsiveContainer no size, so Recharts renders nothing —
  // this only asserts the component mounts without throwing on bad data.
  it('mounts without throwing, coercing bad values to 0', () => {
    const spec = parseChartSpec(
      JSON.stringify({
        type: 'bar',
        title: 'Test',
        x: 'month',
        unit: '€',
        series: [{ key: 'amount' }],
        data: [{ month: 'Jan', amount: 'oops' }, { month: 'Feb', amount: 200 }],
      })
    )
    expect(spec).not.toBeNull()
    expect(() => render(<ChatChart spec={spec!} />)).not.toThrow()
  })
})
