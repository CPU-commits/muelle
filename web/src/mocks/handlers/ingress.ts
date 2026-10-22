import { delay, http, HttpResponse, type RequestHandler } from 'msw'
import { api, apiError, invalid, noContent } from '../api'
import { catalogDb } from '../db/catalog'
import { adminDb } from '../db/admin'
import { appKey, certFields, ingressDb, type MockDomain, NO_TOKEN_REASON } from '../db/ingress'
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

/**
 * Calculados, no guardados (contrato §3.1, `entity.Records`): un A con `public_ip` si hay,
 * y un AAAA con `public_ipv6` si hay, cada uno por su lado, con los ajustes de ahora; `[]`
 * sin ninguna.
 */
function records(hostname: string) {
	const { publicIp, publicIpv6 } = adminDb.state
	return [
		...(publicIp ? [{ type: 'A', name: hostname, content: publicIp }] : []),
		...(publicIpv6 ? [{ type: 'AAAA', name: hostname, content: publicIpv6 }] : []),
	]
}

function domainDto(domain: MockDomain, now = Date.now()) {
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
		// Calculados con los ajustes de ahora (los certificados de Cloudflare, contrato §3.7).
		...certFields(ingressDb.originCa(domain, now)),
		records: records(domain.hostname),
		created_at: iso(domain.createdAt),
		updated_at: iso(domain.updatedAt ?? domain.createdAt),
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

/**
 * En una app de formulario (según el catálogo) el destino es su único servicio, que se
 * llama como la app. En una compose no se verifica al guardar: lo verifica el render del
 * deploy (contrato §3.2).
 */
function formAppSlug(params: Params): string | null {
	const slug = param(params, 'a')
	const config = catalogDb.configured(param(params, 'p'), param(params, 's'), slug)
	return config?.kind === 'service' ? slug : null
}

function checkTarget(
	params: Params,
	body: Record<string, unknown>,
	required: boolean,
): Response | null {
	if (required || body.target_service !== undefined) {
		if (typeof body.target_service !== 'string' || !SERVICE.test(body.target_service)) {
			return invalid('target_service', 'no es un nombre de servicio válido')
		}
		const slug = formAppSlug(params)
		if (slug !== null && body.target_service !== slug) {
			return invalid('target_service', `en una app de formulario tiene que ser "${slug}"`)
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
	// El estado de muelle-caddy (contrato §3.3). Solo con sesión.
	http.get(api('/proxy'), () => requireSession() ?? HttpResponse.json(ingressDb.proxy())),

	http.get(api(DOMAINS), async ({ params }) => {
		const denied = requireSession()
		if (denied) return denied
		const domains = ingressDb.list(param(params, 'p'), param(params, 's'), param(params, 'a'))
		return HttpResponse.json(domains.map((domain) => domainDto(domain)))
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
		const targetProblem = checkTarget(params, body, true)
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
		const targetProblem = checkTarget(params, body, false)
		if (targetProblem) return targetProblem
		if (body.proxied !== undefined) {
			if (typeof body.proxied !== 'boolean')
				return invalid('proxied', 'tiene que ser true o false')
			if (domain.recordId === null) {
				return invalid('proxied', 'muelle no maneja el registro DNS de este dominio')
			}
		}
		await latency()
		const changed = {
			...domain,
			...(typeof body.target_service === 'string'
				? { targetService: body.target_service }
				: {}),
			...(typeof body.target_port === 'number' ? { targetPort: body.target_port } : {}),
			...(typeof body.proxied === 'boolean' ? { proxied: body.proxied } : {}),
		}
		// Si Cloudflare falla al cambiar Proxied, o ya no hay token (se quitó en Ajustes), se
		// guarda lo pedido y el dominio queda en `error` + `cloudflare`: igual responde 200.
		// Si sale bien y estaba en `error` + `cloudflare`, vuelve a `pending`.
		const updated =
			typeof body.proxied !== 'boolean'
				? ingressDb.save(changed)
				: adminDb.state.cloudflareToken === null
					? ingressDb.failCloudflare(changed, NO_TOKEN_REASON)
					: ingressDb.cloudflareFlaky(domain)
						? ingressDb.failCloudflare(changed)
						: ingressDb.recoverCloudflare(changed)
		return HttpResponse.json(domainDto(updated))
	}),

	http.delete(api(`${DOMAINS}/:hostname`), async ({ params }) => {
		const denied = requireSession()
		if (denied) return denied
		const hostname = param(params, 'hostname')
		const domain = ingressDb.find(keyOf(params), hostname)
		if (!domain) return domainNotFound(hostname)
		await latency()
		if (ingressDb.cloudflareDown(domain)) {
			return apiError(
				502,
				'cloudflare_unavailable',
				'Cloudflare no respondió: no se borró nada',
			)
		}
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
		// Cloudflare falla al adoptar: 200 con el dominio en `error` + `cloudflare`.
		if (ingressDb.cloudflareFlaky(domain)) {
			return HttpResponse.json(domainDto(ingressDb.failCloudflare(domain)))
		}
		return HttpResponse.json(domainDto(ingressDb.adopt(domain)))
	}),

	http.post(api(`${DOMAINS}/:hostname/retry`), async ({ params }) => {
		const denied = requireSession()
		if (denied) return denied
		const hostname = param(params, 'hostname')
		const domain = ingressDb.find(keyOf(params), hostname)
		if (!domain) return domainNotFound(hostname)
		await latency()
		// La respuesta es de la hora del pedido: el certificado de Cloudflare que falló dice
		// que se reintenta ahora (`retry_at` en el pasado), y el resultado se ve en un GET.
		const now = Date.now()
		return HttpResponse.json(domainDto(ingressDb.retry(domain, now), now))
	}),
]
