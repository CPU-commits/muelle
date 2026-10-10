import { QueryClient } from '@tanstack/svelte-query'
import { type DomainErrorCode, isDomainError, pendingPhase } from '@/shared/domain'
import { ApiError } from '@/shared/data/http/errors'

const NON_RETRYABLE_STATUS = [400, 401, 403, 404, 409, 422, 423, 429]

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
]

// Un 4xx no se arregla reintentando, ni un endpoint de una fase que todavía no existe.
// Solo se reintentan errores de red y 5xx.
export function shouldRetry(failureCount: number, error: unknown): boolean {
	if (pendingPhase(error) !== null) return false
	if (isDomainError(error) && NON_RETRYABLE_CODES.includes(error.code)) return false
	if (error instanceof ApiError && NON_RETRYABLE_STATUS.includes(error.status)) return false
	return failureCount < 2
}

// Un único QueryClient de módulo es seguro porque la app es SPA: no hay requests de
// servidor concurrentes que puedan compartir cache entre usuarios. Al cerrar sesión se
// vacía (`queryClient.clear()`), para que el siguiente usuario no vea datos del anterior.
export const queryClient = new QueryClient({
	defaultOptions: {
		queries: {
			staleTime: 30_000,
			refetchOnWindowFocus: false,
			retry: shouldRetry,
		},
		mutations: { retry: false },
	},
})
