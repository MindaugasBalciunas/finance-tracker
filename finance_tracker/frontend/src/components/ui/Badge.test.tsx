import { render, screen } from '@testing-library/react'
import { describe, it, expect } from 'vitest'
import Badge from './Badge'

describe('Badge', () => {
  it('renders expense badge', () => {
    render(<Badge type="expense" />)
    expect(screen.getByText('expense')).toBeInTheDocument()
  })

  it('renders income badge', () => {
    render(<Badge type="income" />)
    expect(screen.getByText('income')).toBeInTheDocument()
  })

  it('renders investment badge', () => {
    render(<Badge type="investment" />)
    expect(screen.getByText('investment')).toBeInTheDocument()
  })

  it('applies red color for expense', () => {
    render(<Badge type="expense" />)
    expect(screen.getByText('expense')).toHaveClass('bg-red-100')
  })

  it('applies green color for income', () => {
    render(<Badge type="income" />)
    expect(screen.getByText('income')).toHaveClass('bg-green-100')
  })
})
