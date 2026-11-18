import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import 'normalize.css'
import './index.css'
import App from './App.tsx'
import Sweep from './game/Sweep.tsx'
import {flags} from './flags'

// ?sweep is the M-A harness (01 Thesis/Measurement Methodology.md) and it drives the sim itself, so
// it replaces the game rather than running beside it.
const page = flags.sweep ? <Sweep /> : <App />

createRoot(document.getElementById('root')!).render(<StrictMode>{page}</StrictMode>)
