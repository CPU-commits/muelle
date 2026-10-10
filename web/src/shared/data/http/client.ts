import { config } from '@/shared/data/config'
import { type ApiError, apiErrorFromResponse, NetworkError } from './errors'

export const API_PREFIX = '/api/v1'

export type HttpMethod = 'GET' | 'POST' | 'PUT' | 'PATCH' | 'DELETE'

export type QueryParams = Record<string, string | number | boolean | null | undefined>

export interface HttpOptions {
	query?: QueryParams
	body?: unknown
	signal?: AbortSignal
	/**
	 * `false` en los endpoints de acceso (login, código, setup, invitación, me): ahí un 401
	 * es una respuesta esperada que maneja la pantalla, no una sesión que venció. Por
	 * defecto `true`: un 401 manda al login (ver `configureHttp`).
	 */
	redirectOn401?: boolean
}

// El cliente no conoce el router: recibe qué hacer al arrancar la app (`configureHttp`
// en hooks.client). Así `shared/data` no depende de `shared/ui` y los tests de
// integración corren en node.
interface HttpHooks {
	/** Un 401 en una ruta con sesión: venció, la cerraron o falta el código de 2FA. */
	onUnauthorized: (error: ApiError) => void
	/** Un 403 `totp_setup_required`: el panel exige 2FA y este usuario todavía no la tiene. */
	onTotpSetupRequired: (error: ApiError) => void
}

const hooks: HttpHooks = {
	onUnauthorized: () => {},
	onTotpSetupRequired: () => {},
}

export function configureHttp(overrides: Partial<HttpHooks>): void {
	if (overrides.onUnauthorized) hooks.onUnauthorized = overrides.onUnauthorized
	if (overrides.onTotpSetupRequired) hooks.onTotpSetupRequired = overrides.onTotpSetupRequired
}

export function apiUrl(path: string, query?: QueryParams): string {
	const url = `${config.apiUrl}${API_PREFIX}${path}`
	if (!query) return url

	const search = new URLSearchParams()
	for (const [key, value] of Object.entries(query)) {
		if (value === undefined || value === null || value === '') continue
		search.set(key, String(value))
	}
	const qs = search.toString()
	return qs ? `${url}?${qs}` : url
}

function parseJson(raw: string): unknown {
	try {
		return JSON.parse(raw)
	} catch {
		return raw
	}
}

async function request<T>(method: HttpMethod, path: string, options: HttpOptions = {}): Promise<T> {
	const headers: Record<string, string> = { Accept: 'application/json' }

	// La sesión es la cookie `muelle_session` (HttpOnly): el navegador la manda sola en
	// una petición del mismo origen, junto con el `Origin` que el backend compara.
	const init: RequestInit = { method, headers, credentials: 'same-origin' }
	if (options.body !== undefined) {
		headers['Content-Type'] = 'application/json'
		init.body = JSON.stringify(options.body)
	}
	if (options.signal) init.signal = options.signal

	let response: Response
	try {
		response = await fetch(apiUrl(path, options.query), init)
	} catch (cause) {
		if (cause instanceof DOMException && cause.name === 'AbortError') throw cause
		throw new NetworkError(cause)
	}

	const raw = await response.text()
	const payload: unknown = raw ? parseJson(raw) : null

	if (response.ok) return payload as T

	const error = apiErrorFromResponse(response.status, payload)
	if (options.redirectOn401 !== false) {
		if (error.status === 401) hooks.onUnauthorized(error)
		else if (error.status === 403 && error.code === 'totp_setup_required') {
			hooks.onTotpSetupRequired(error)
		}
	}
	throw error
}

export const http = {
	get: <T>(path: string, options?: HttpOptions) => request<T>('GET', path, options),
	post: <T>(path: string, options?: HttpOptions) => request<T>('POST', path, options),
	put: <T>(path: string, options?: HttpOptions) => request<T>('PUT', path, options),
	patch: <T>(path: string, options?: HttpOptions) => request<T>('PATCH', path, options),
	delete: <T>(path: string, options?: HttpOptions) => request<T>('DELETE', path, options),
}
