import { render, screen } from '@testing-library/react'
import { describe, it, expect } from 'vitest'
import StatCard from './StatCard'

describe('StatCard', () => {
  it('renders title and value', () => {
    render(<StatCard title="Net Worth" value="€36,000" />)
    expect(screen.getByText('Net Worth')).toBeInTheDocument()
    expect(screen.getByText('€36,000')).toBeInTheDocument()
  })

  it('renders optional subtitle', () => {
    render(<StatCard title="Total" value="€1,000" subtitle="All accounts" />)
    expect(screen.getByText('All accounts')).toBeInTheDocument()
  })

  it('renders positive trend', () => {
    render(<StatCard title="Total" value="€1,000" trend={5.2} />)
    expect(screen.getByText(/5\.2%/)).toBeInTheDocument()
    expect(screen.getByText(/▲/)).toBeInTheDocument()
  })

  it('renders negative trend', () => {
    render(<StatCard title="Total" value="€1,000" trend={-3.1} />)
    expect(screen.getByText(/3\.1%/)).toBeInTheDocument()
    expect(screen.getByText(/▼/)).toBeInTheDocument()
  })
})
