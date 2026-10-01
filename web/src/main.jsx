import { render } from 'preact'
import { App } from './app.jsx'
import { getTheme } from './theme.js'
import './style.css'

document.documentElement.dataset.theme = getTheme()
render(<App />, document.getElementById('app'))
