import { createContext, useContext, useState } from 'react'
import type { DateRange } from '../components/ui/DateRangeFilter'

interface DateRangeContextValue {
  dateRange: DateRange
  setDateRange: (r: DateRange) => void
}

const DateRangeContext = createContext<DateRangeContextValue>({
  dateRange: {},
  setDateRange: () => {},
})

export function DateRangeProvider({ children }: { children: React.ReactNode }) {
  const [dateRange, setDateRange] = useState<DateRange>({})
  return (
    <DateRangeContext.Provider value={{ dateRange, setDateRange }}>
      {children}
    </DateRangeContext.Provider>
  )
}

export function useDateRange() {
  return useContext(DateRangeContext)
}
