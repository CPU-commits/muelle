import { HttpResponse } from 'msw'
import { config } from '@/shared/data/config'
import { API_PREFIX } from '@/shared/data/http/client'

// La URL que intercepta un handler: `api('/projects')` → `/api/v1/projects` (o con el
// origen de los tests de node). Los handlers usan esto, nunca la ruta a mano.
export function api(path: string): string {
	return `${config.apiUrl}${API_PREFIX}${path}`
}

// El sobre de error del backend (spec §10): `{"error": {"code", "message"}}`.
export function apiError(
	status: number,
	code: string,
	message: string,
): HttpResponse<{
	error: { code: string; message: string }
}> {
	return HttpResponse.json({ error: { code, message } }, { status })
}

// Un 422 de validación como lo arma el backend: `"<campo>: <motivo>"`.
export function invalid(field: string, reason: string) {
	return apiError(422, 'invalid', `${field}: ${reason}`)
}

/** Un recurso que no existe (un proyecto, un deploy). */
export function notFound() {
	return apiError(404, 'not_found', 'no existe')
}

/**
 * Una ruta que no existe, como responde el backend a lo que todavía no implementa (Fase
 * 2/3). `handlers/index.ts` la devuelve para toda ruta de la API sin handler.
 */
export function noRoute() {
	return apiError(404, 'no_route', 'no existe la ruta')
}

export function noContent(): HttpResponse<null> {
	return new HttpResponse(null, { status: 204 })
}
