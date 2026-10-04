import React from 'react'
import ReactDOM from 'react-dom/client'
import App, { applyTheme } from './App'
import './index.css'

applyTheme()
ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <App />
  </React.StrictMode>,
)
