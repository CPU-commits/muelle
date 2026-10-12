import { delay, http, HttpResponse, type RequestHandler } from 'msw'
import { api, apiError, invalid, noContent } from '../api'
import { appKey, ingressDb, type MockDomain, MOCK_PUBLIC_IP } from '../db/ingress'
import { requireSession } from '../session'

// Dominios (Fase 3) del backend simulado, con el contrato de
// src/modules/ingress/README.md. Las reglas del mock están en ../db/ingress.ts.

const DOMAINS = '/projects/:p/stages/:s/apps/:a/domains'

type Params = Record<string, string | readonly string[] | undefined>

function param(params: Params, name: string): string {
	const value = params[name]
	return Array.isArray(value) ? String(value[0] ?? '') : String(value ?? '')
}

function keyOf(params: Params): string {
	return appKey(param(params, 'p'), param(params, 's'), param(params, 'a'))
}

function iso(ms: number | null): string | null {
	return ms === null ? null : new Date(ms).toISOString()
}

function domainDto(domain: MockDomain) {
	return {
		hostname: domain.hostname,
		target_service: domain.targetService,
		target_port: domain.targetPort,
		proxied: domain.proxied,
		tls_mode: domain.tlsMode,
		cf_zone_id: domain.zoneId,
		cf_record_id: domain.recordId,
		status: domain.status,
		last_error: domain.lastError,
		last_error_code: domain.lastErrorCode,
		cert_issuer: domain.certIssuer,
		cert_expires_at: iso(domain.certExpiresAt),
		records: [{ type: 'A', name: domain.hostname, content: MOCK_PUBLIC_IP }],
		created_at: iso(domain.createdAt),
	}
}

const HOSTNAME =
	/^(?=.{1,253}$)([a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z]([a-z0-9-]{0,61}[a-z0-9])?$/
const SERVICE = /^[a-z]([a-z0-9-]{0,14}[a-z0-9])?$/

const latency = () => delay(120)

function domainNotFound(hostname: string) {
	return apiError(404, 'not_found', `no existe el dominio ${hostname}`)
}

async function readBody(request: Request): Promise<Record<string, unknown> | Response> {
	try {
		const body: unknown = await request.json()
		if (typeof body === 'object' && body !== null && !Array.isArray(body)) {
			return body as Record<string, unknown>
		}
	} catch {
		// Cae al 400 de abajo.
	}
	return apiError(400, 'invalid_json', 'el cuerpo tiene que ser un objeto JSON')
}

function checkTarget(body: Record<string, unknown>, required: boolean): Response | null {
	if (required || body.target_service !== undefined) {
		if (typeof body.target_service !== 'string' || !SERVICE.test(body.target_service)) {
			return invalid('target_service', 'no es un nombre de servicio válido')
		}
	}
	if (required || body.target_port !== undefined) {
		const port = body.target_port
		if (typeof port !== 'number' || !Number.isInteger(port) || port < 1 || port > 65535) {
			return invalid('target_port', 'tiene que ser un entero entre 1 y 65535')
		}
	}
	return null
}

export const ingressHandlers: RequestHandler[] = [
	http.get(api(DOMAINS), async ({ params }) => {
		const denied = requireSession()
		if (denied) return denied
		const domains = ingressDb.list(param(params, 'p'), param(params, 's'), param(params, 'a'))
		return HttpResponse.json(domains.map(domainDto))
	}),

	http.post(api(DOMAINS), async ({ params, request }) => {
		const denied = requireSession()
		if (denied) return denied
		const body = await readBody(request)
		if (body instanceof Response) return body
		const unknownKey = Object.keys(body).find(
			(key) => !['hostname', 'target_service', 'target_port', 'proxied'].includes(key),
		)
		if (unknownKey) return apiError(400, 'invalid_json', `campo desconocido "${unknownKey}"`)
		const hostname = typeof body.hostname === 'string' ? body.hostname : ''
		if (!HOSTNAME.test(hostname)) return invalid('hostname', 'no es un hostname válido')
		const targetProblem = checkTarget(body, true)
		if (targetProblem) return targetProblem
		if (body.proxied !== undefined && typeof body.proxied !== 'boolean') {
			return invalid('proxied', 'tiene que ser true o false')
		}
		await latency()
		if (ingressDb.findAnywhere(hostname)) {
			return apiError(409, 'conflict', 'ya existe')
		}
		// Sembrar la app antes, para que el nuevo no quede solo en la lista.
		ingressDb.list(param(params, 'p'), param(params, 's'), param(params, 'a'))
		const created = ingressDb.create(keyOf(params), {
			hostname,
			targetService: body.target_service as string,
			targetPort: body.target_port as number,
			proxied: body.proxied === true,
		})
		return HttpResponse.json(domainDto(created), { status: 201 })
	}),

	http.patch(api(`${DOMAINS}/:hostname`), async ({ params, request }) => {
		const denied = requireSession()
		if (denied) return denied
		const hostname = param(params, 'hostname')
		const domain = ingressDb.find(keyOf(params), hostname)
		if (!domain) return domainNotFound(hostname)
		const body = await readBody(request)
		if (body instanceof Response) return body
		const unknownKey = Object.keys(body).find(
			(key) => !['target_service', 'target_port', 'proxied'].includes(key),
		)
		if (unknownKey) return apiError(400, 'invalid_json', `campo desconocido "${unknownKey}"`)
		const targetProblem = checkTarget(body, false)
		if (targetProblem) return targetProblem
		if (body.proxied !== undefined) {
			if (typeof body.proxied !== 'boolean')
				return invalid('proxied', 'tiene que ser true o false')
			if (domain.recordId === null) {
				return invalid('proxied', 'muelle no maneja el registro DNS de este dominio')
			}
		}
		await latency()
		const updated = ingressDb.save({
			...domain,
			...(typeof body.target_service === 'string'
				? { targetService: body.target_service }
				: {}),
			...(typeof body.target_port === 'number' ? { targetPort: body.target_port } : {}),
			...(typeof body.proxied === 'boolean' ? { proxied: body.proxied } : {}),
		})
		return HttpResponse.json(domainDto(updated))
	}),

	http.delete(api(`${DOMAINS}/:hostname`), async ({ params }) => {
		const denied = requireSession()
		if (denied) return denied
		const hostname = param(params, 'hostname')
		if (!ingressDb.find(keyOf(params), hostname)) return domainNotFound(hostname)
		await latency()
		ingressDb.remove(keyOf(params), hostname)
		return noContent()
	}),

	http.post(api(`${DOMAINS}/:hostname/adopt`), async ({ params }) => {
		const denied = requireSession()
		if (denied) return denied
		const hostname = param(params, 'hostname')
		const domain = ingressDb.find(keyOf(params), hostname)
		if (!domain) return domainNotFound(hostname)
		if (domain.lastErrorCode !== 'record_exists' || domain.recordId !== null) {
			return apiError(409, 'nothing_to_adopt', 'no hay un registro existente que adoptar')
		}
		await latency()
		return HttpResponse.json(domainDto(ingressDb.adopt(domain)))
	}),

	http.post(api(`${DOMAINS}/:hostname/retry`), async ({ params }) => {
		const denied = requireSession()
		if (denied) return denied
		const hostname = param(params, 'hostname')
		const domain = ingressDb.find(keyOf(params), hostname)
		if (!domain) return domainNotFound(hostname)
		await latency()
		return HttpResponse.json(domainDto(ingressDb.retry(domain)))
	}),
]
