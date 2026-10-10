import { setupServer } from 'msw/node'
import { handlers } from './handlers'

// Para los tests de integración en node: `server.listen({ onUnhandledRequest: 'error' })`.
export const server = setupServer(...handlers)
