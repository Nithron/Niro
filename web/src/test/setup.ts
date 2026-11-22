import '@testing-library/jest-dom/vitest'
import { cleanup } from '@testing-library/react'
import { afterEach } from 'vitest'

// Testing Library only auto-cleans when Vitest globals are enabled; we don't
// use globals, so unmount after every test explicitly.
afterEach(cleanup)
