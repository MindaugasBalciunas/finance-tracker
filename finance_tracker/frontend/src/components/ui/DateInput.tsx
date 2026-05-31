import DatePicker from 'react-datepicker'
import { parseISO, isValid, format } from 'date-fns'
import 'react-datepicker/dist/react-datepicker.css'

interface Props {
  value: string           // yyyy-MM-dd or ''
  onChange: (val: string) => void
  className?: string
  required?: boolean
}

const INPUT_CLASS =
  'w-full border border-gray-300 rounded-lg px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-blue-500'

export default function DateInput({ value, onChange, className, required }: Props) {
  const parsed = value ? parseISO(value) : null
  const selected = parsed && isValid(parsed) ? parsed : null

  return (
    <DatePicker
      selected={selected}
      onChange={(date: Date | null) => onChange(date ? format(date, 'yyyy-MM-dd') : '')}
      dateFormat="yyyy-MM-dd"
      calendarStartDay={1}
      wrapperClassName="w-full"
      className={className ?? INPUT_CLASS}
      required={required}
      showMonthDropdown
      showYearDropdown
      dropdownMode="select"
    />
  )
}
