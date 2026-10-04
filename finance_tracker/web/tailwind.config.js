/** @type {import('tailwindcss').Config} */
const v = (name) => `rgb(var(--${name}) / <alpha-value>)`
export default {
  content: ['./index.html', './src/**/*.{ts,tsx}'],
  darkMode: ['class', '[data-theme="dark"]'],
  theme: {
    extend: {
      colors: {
        page: v('page'), surface: v('surface'), raised: v('raised'), sunken: v('sunken'),
        ink: v('ink'), ink2: v('ink2'), muted: v('muted'), line: v('line'), axis: v('axis'),
        accent: v('accent'), good: v('good'), bad: v('bad'), warn: v('warn'),
      },
      fontFamily: { sans: ['system-ui', '-apple-system', '"Segoe UI"', 'Roboto', 'sans-serif'] },
      borderRadius: { xl: '14px', '2xl': '18px' },
    },
  },
  plugins: [],
}
