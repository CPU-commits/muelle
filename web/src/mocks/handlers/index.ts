import { http, type RequestHandler } from 'msw'
import { api, noRoute } from '../api'
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
	// Lo que nadie mockea responde como el backend a una ruta que no existe: 404 `no_route`
	// (un repositorio de Fase 2/3 lo lee como "todavía no existe"). Va último.
	http.all(api('/*'), () => noRoute()),
]
