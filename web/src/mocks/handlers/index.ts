import type { RequestHandler } from 'msw'
import { adminHandlers } from './admin'
import { authHandlers } from './auth'
import { catalogHandlers } from './catalog'
import { deployHandlers } from './deploy'
import { identityHandlers } from './identity'
import { ingressHandlers } from './ingress'
import { secretsHandlers } from './secrets'

// Todos los handlers del backend simulado, un archivo por módulo (`handlers/<módulo>.ts`
// con sus datos en `db/<módulo>.ts`). Cada tarea edita solo los suyos: este archivo ya
// los importa a todos.
export const handlers: RequestHandler[] = [
	...authHandlers,
	...catalogHandlers,
	...secretsHandlers,
	...identityHandlers,
	...adminHandlers,
	...deployHandlers,
	...ingressHandlers,
]
