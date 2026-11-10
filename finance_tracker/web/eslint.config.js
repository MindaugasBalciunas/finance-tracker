// Lint for the web app: TypeScript's recommended rules plus the React hooks
// rules the code already assumed (several `eslint-disable-next-line
// react-hooks/exhaustive-deps` comments were there before ESLint was).
// tsc --noEmit still does the type checking; this catches what types can't —
// a hook called conditionally, a stale closure in an effect, `any` creeping in.
import js from '@eslint/js'
import globals from 'globals'
import reactHooks from 'eslint-plugin-react-hooks'
import tseslint from 'typescript-eslint'

export default tseslint.config(
  { ignores: ['dist', 'node_modules'] },
  {
    files: ['src/**/*.{ts,tsx}'],
    extends: [js.configs.recommended, ...tseslint.configs.recommended],
    languageOptions: { ecmaVersion: 2022, globals: globals.browser },
    plugins: { 'react-hooks': reactHooks },
    rules: {
      'react-hooks/rules-of-hooks': 'error',
      'react-hooks/exhaustive-deps': 'warn',
      // tsc's noUnusedLocals/Parameters already fail the build on these.
      '@typescript-eslint/no-unused-vars': 'off',
      // `try { localStorage… } catch {}`: storage can throw (private mode, blocked site data) and there's nothing to do.
      'no-empty': ['error', { allowEmptyCatch: true }],
      // Short-circuit statements (`a && b()`, `s.has(x) ? s.delete(x) : s.add(x)`) are this codebase's idiom.
      '@typescript-eslint/no-unused-expressions': ['error', { allowShortCircuit: true, allowTernary: true }],
    },
  },
  {
    // Tests stub fetch and recharts with loose shapes on purpose.
    files: ['src/test/**'],
    rules: { '@typescript-eslint/no-explicit-any': 'off' },
  },
)
