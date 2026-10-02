import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, it, expect, vi } from 'vitest'
import AddTransactionButton from './AddTransactionButton'

function setup(over: Partial<Parameters<typeof AddTransactionButton>[0]> = {}) {
  const props = {
    onManual: vi.fn(),
    onScan: vi.fn(),
    onBank: vi.fn(),
    scanAvailable: true,
    bankPending: 0,
    ...over,
  }
  render(<AddTransactionButton {...props} />)
  return props
}

describe('AddTransactionButton', () => {
  // Typing it in, photographing it and pulling it from the bank are the same
  // job done three ways, so they belong behind the same button.
  it('offers all three ways in', async () => {
    const user = userEvent.setup()
    setup()

    await user.click(screen.getByRole('button', { name: '+ Add' }))
    expect(screen.getByRole('menuitem', { name: /Enter manually/ })).toBeInTheDocument()
    expect(screen.getByRole('menuitem', { name: /Scan a photo/ })).toBeInTheDocument()
    expect(screen.getByRole('menuitem', { name: /From your bank/ })).toBeInTheDocument()
  })

  it('opens the form for a manual entry', async () => {
    const user = userEvent.setup()
    const props = setup()

    await user.click(screen.getByRole('button', { name: '+ Add' }))
    await user.click(screen.getByRole('menuitem', { name: /Enter manually/ }))
    expect(props.onManual).toHaveBeenCalled()
  })

  // The picker is opened from inside the click, not from an effect after a
  // modal mounts — iOS blocks the latter.
  it('hands the picked photo straight to the caller', async () => {
    const user = userEvent.setup()
    const props = setup()
    const file = new File(['x'], 'receipt.jpg', { type: 'image/jpeg' })

    await user.click(screen.getByRole('button', { name: '+ Add' }))
    const input = document.querySelector('input[type="file"]') as HTMLInputElement
    await user.upload(input, file)

    expect(props.onScan).toHaveBeenCalledWith(file)
  })

  it('shows how many bank rows are waiting', async () => {
    const user = userEvent.setup()
    setup({ bankPending: 4 })

    await user.click(screen.getByRole('button', { name: '+ Add' }))
    expect(screen.getByText('4 waiting to review')).toBeInTheDocument()
  })

  // The bank and AI answers arrive from queries, so the number of options
  // flips from 1 to 3 a moment after the page loads. That must not disturb a
  // menu the user has already opened — it used to remount the component and
  // close it silently.
  it('keeps the menu open when the options arrive late', async () => {
    const user = userEvent.setup()
    const props = {
      onManual: vi.fn(),
      onScan: vi.fn(),
      onBank: undefined as (() => void) | undefined,
      scanAvailable: false,
    }
    const { rerender } = render(<AddTransactionButton {...props} />)

    rerender(<AddTransactionButton {...props} scanAvailable onBank={vi.fn()} />)
    await user.click(screen.getByRole('button', { name: '+ Add' }))
    expect(screen.getByRole('menu')).toBeInTheDocument()

    // A later settings refetch re-renders with the same options.
    rerender(<AddTransactionButton {...props} scanAvailable onBank={vi.fn()} />)
    expect(screen.getByRole('menu')).toBeInTheDocument()
  })

  // With AI off and no bank connected there is only one way in, so a menu
  // would be a pointless extra tap.
  it('skips the menu when typing is the only way in', async () => {
    const user = userEvent.setup()
    const props = setup({ scanAvailable: false, onBank: undefined })

    await user.click(screen.getByRole('button', { name: '+ Add' }))
    expect(props.onManual).toHaveBeenCalled()
    expect(screen.queryByRole('menu')).not.toBeInTheDocument()
  })
})
