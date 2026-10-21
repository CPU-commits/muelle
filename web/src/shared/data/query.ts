import { type DefaultOptions, QueryClient } from '@tanstack/svelte-query'
import { type DomainErrorCode, isDomainError, UnavailableError } from '@/shared/domain'
import { ApiError, ContractError } from '@/shared/data/http/errors'

// Cuando una query falla, lo que llega acá NO es un ApiError: el repositorio ya lo
// tradujo a un error de dominio (un 404 es un `ProjectNotFound`). Mirar solo el status
// HTTP haría reintentar un "no existe" dos veces, con backoff, antes de mostrarlo.
const NON_RETRYABLE_CODES: readonly DomainErrorCode[] = [
	'validation',
	'not_found',
	'unauthorized',
	'forbidden',
	'conflict',
	'rate_limited',
	// Lo que no es 5xx ni red (un 400 `invalid_json`, un 410): reintentar da lo mismo.
	'unexpected',
]

// Un 4xx no se arregla reintentando, ni una respuesta con otra forma (ContractError: el
// backend cambió). Solo se reintentan errores de red y 5xx.
export function shouldRetry(failureCount: number, error: unknown): boolean {
	// Un timeout ya esperó 30 s: reintentarlo dos veces deja la pantalla cargando 90 s.
	if (error instanceof UnavailableError && error.reason === 'timeout') return false
	if (error instanceof ContractError) return false
	if (isDomainError(error) && NON_RETRYABLE_CODES.includes(error.code)) return false
	if (error instanceof ApiError && error.status >= 400 && error.status < 500) return false
	return failureCount < 2
}

type QueryDefaults = NonNullable<DefaultOptions['queries']>

const QUERY_DEFAULTS: QueryDefaults = {
	staleTime: 30_000,
	refetchOnWindowFocus: false,
	retry: shouldRetry,
	// Por defecto TanStack solo avisa cambios de las propiedades que ya se leyeron. Un
	// viewModel que mira `data` primero (y `error` recién cuando deja de "cargar") no se
	// enteraba del error: `data` no cambia, nadie avisa, y la pantalla quedaba cargando.
	// Con 'all' cualquier cambio del resultado llega.
	notifyOnChangeProps: 'all',
}

/**
 * Un QueryClient con las opciones del panel. Los tests de viewModels usan este, con lo que
 * cambie el test encima: `createQueryClient({ retry: false })`.
 */
export function createQueryClient(queries: QueryDefaults = {}): QueryClient {
	return new QueryClient({
		defaultOptions: {
			queries: { ...QUERY_DEFAULTS, ...queries },
			mutations: { retry: false },
		},
	})
}

// Un único QueryClient de módulo es seguro porque la app es SPA: no hay requests de
// servidor concurrentes que puedan compartir cache entre usuarios. Al cerrar sesión se
// vacía (`queryClient.clear()`), para que el siguiente usuario no vea datos del anterior.
export const queryClient = createQueryClient()
