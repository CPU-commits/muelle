// Un error HTTP de la API de muelle. El backend siempre responde
// `{"error": {"code": "…", "message": "…"}}` (spec §10): `code` es lo que se mira para
// decidir; `message`, en español, es para logs y como último recurso.
export class ApiError extends Error {
	readonly status: number
	/** El `error.code` del backend (`invalid_credentials`, `not_found`…); `unknown` si no vino. */
	readonly code: string
	readonly payload: unknown

	constructor(status: number, code: string, message: string, payload: unknown = null) {
		super(message)
		this.name = 'ApiError'
		this.status = status
		this.code = code
		this.payload = payload
	}
}

export const UNKNOWN_ERROR_CODE = 'unknown'

interface ErrorEnvelope {
	error: { code: string; message: string }
}

function isErrorEnvelope(payload: unknown): payload is ErrorEnvelope {
	if (typeof payload !== 'object' || payload === null) return false
	const error = (payload as { error?: unknown }).error
	if (typeof error !== 'object' || error === null) return false
	const { code, message } = error as { code?: unknown; message?: unknown }
	return typeof code === 'string' && typeof message === 'string'
}

// Lo que no trae el sobre (un 502 del proxy de Vite, un HTML de error) queda con
// `code: 'unknown'` y se clasifica solo por status.
export function apiErrorFromResponse(status: number, payload: unknown): ApiError {
	if (isErrorEnvelope(payload)) {
		return new ApiError(status, payload.error.code, payload.error.message, payload)
	}
	return new ApiError(status, UNKNOWN_ERROR_CODE, `HTTP ${status}`, payload)
}

export class NetworkError extends Error {
	constructor(cause: unknown) {
		super('No se pudo contactar al servidor', { cause })
		this.name = 'NetworkError'
	}
}

export class ContractError extends Error {
	readonly detail: string

	constructor(resource: string, detail: string) {
		super(`Respuesta inesperada del backend para "${resource}"`)
		this.name = 'ContractError'
		this.detail = detail
	}
}

export function isApiError(error: unknown): error is ApiError {
	return error instanceof ApiError
}

export function apiErrorStatus(error: unknown): number | null {
	return isApiError(error) ? error.status : null
}

export function apiErrorCode(error: unknown): string | null {
	return isApiError(error) ? error.code : null
}
