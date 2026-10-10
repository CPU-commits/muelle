import { http, HttpResponse, type RequestHandler } from 'msw'
import { api, apiError, invalid, noContent, notFound } from '../api'
import { adminDb, type MockRegistry } from '../db/admin'
import { authDb } from '../db/auth'
import { requireSession, sessionUser } from '../session'

// `/settings` y `/audit` (admin/infra/in/http) y, con los contratos del spec, lo que
// agregan las Fases 2 y 3: los campos de proxy/DNS de `/settings` y `/registries`. Contra
// el backend real de la Fase 1 esos campos faltan y `/registries` responde 404.

const MAX_AUDIT_LIMIT = 500
const DEFAULT_AUDIT_LIMIT = 100

const IPV4 = /^((25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)\.){3}(25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)$/
const IPV6 = /^[0-9a-f:.]*:[0-9a-f:.]*$/i
const CIDR = /^[0-9a-f:.]+(\/\d{1,3})?$/i
const EMAIL = /^[^\s@]+@[^\s@]+\.[^\s@]{2,}$/

function settingsRes() {
	const s = adminDb.state
	return {
		require_2fa: authDb.state.require2fa,
		cloudflare_token_set: s.cloudflareToken !== null,
		cloudflare_zones: s.cloudflareToken !== null ? s.cloudflareZones : null,
		public_ip: s.publicIp,
		public_ipv6: s.publicIpv6,
		acme_email: s.acmeEmail,
		panel_domain: s.panelDomain,
		proxy_http_port: s.proxyHttpPort,
		proxy_https_port: s.proxyHttpsPort,
		panel_allowed_cidrs: s.panelAllowedCidrs,
		client_ip: s.clientIp,
	}
}

interface SettingsBody {
	require_2fa?: unknown
	public_ip?: unknown
	public_ipv6?: unknown
	acme_email?: unknown
	proxy_http_port?: unknown
	proxy_https_port?: unknown
	panel_allowed_cidrs?: unknown
	cloudflare_token?: unknown
}

const SETTINGS_FIELDS = new Set([
	'require_2fa',
	'public_ip',
	'public_ipv6',
	'acme_email',
	'proxy_http_port',
	'proxy_https_port',
	'panel_allowed_cidrs',
	'cloudflare_token',
])

function isPort(value: unknown): value is number {
	return typeof value === 'number' && Number.isInteger(value) && value >= 1 && value <= 65535
}

function validateSettings(body: SettingsBody) {
	if (body.require_2fa !== undefined && typeof body.require_2fa !== 'boolean') {
		return apiError(400, 'invalid_json', 'require_2fa debe ser un booleano')
	}
	if (body.public_ip !== undefined && !IPV4.test(String(body.public_ip))) {
		return invalid('public_ip', 'no es una IPv4 válida')
	}
	if (body.public_ipv6 !== undefined && body.public_ipv6 !== '') {
		if (!IPV6.test(String(body.public_ipv6)))
			return invalid('public_ipv6', 'no es una IPv6 válida')
	}
	if (body.acme_email !== undefined && !EMAIL.test(String(body.acme_email))) {
		return invalid('acme_email', 'no es un email válido')
	}
	if (body.proxy_http_port !== undefined && !isPort(body.proxy_http_port)) {
		return invalid('proxy_http_port', 'debe estar entre 1 y 65535')
	}
	if (body.proxy_https_port !== undefined && !isPort(body.proxy_https_port)) {
		return invalid('proxy_https_port', 'debe estar entre 1 y 65535')
	}
	const http = body.proxy_http_port ?? adminDb.state.proxyHttpPort
	const https = body.proxy_https_port ?? adminDb.state.proxyHttpsPort
	if (http === https) return invalid('proxy_https_port', 'no puede ser el mismo que HTTP')
	if (body.panel_allowed_cidrs !== undefined) {
		const list = body.panel_allowed_cidrs
		if (!Array.isArray(list)) return apiError(400, 'invalid_json', 'debe ser una lista')
		const bad = list.find((entry) => typeof entry !== 'string' || !CIDR.test(entry))
		if (bad !== undefined) {
			return invalid('panel_allowed_cidrs', `"${String(bad)}" no es un rango CIDR`)
		}
	}
	if (body.cloudflare_token !== undefined && String(body.cloudflare_token).trim() === '') {
		return invalid('cloudflare_token', 'es obligatorio')
	}
	return undefined
}

function applySettings(body: SettingsBody): string[] {
	const s = adminDb.state
	if (typeof body.require_2fa === 'boolean') authDb.state.require2fa = body.require_2fa
	if (typeof body.public_ip === 'string') s.publicIp = body.public_ip
	if (typeof body.public_ipv6 === 'string') s.publicIpv6 = body.public_ipv6
	if (typeof body.acme_email === 'string') s.acmeEmail = body.acme_email
	if (isPort(body.proxy_http_port)) s.proxyHttpPort = body.proxy_http_port
	if (isPort(body.proxy_https_port)) s.proxyHttpsPort = body.proxy_https_port
	if (Array.isArray(body.panel_allowed_cidrs)) {
		s.panelAllowedCidrs = body.panel_allowed_cidrs.map(String)
	}
	if (typeof body.cloudflare_token === 'string') s.cloudflareToken = body.cloudflare_token
	authDb.commit()
	adminDb.commit()
	return Object.keys(body)
}

function registryRes(registry: MockRegistry) {
	return { host: registry.host, username: registry.username }
}

interface RegistryBody {
	host?: unknown
	username?: unknown
	password?: unknown
}

async function readJson<T>(request: Request): Promise<T | null> {
	try {
		return (await request.json()) as T
	} catch {
		return null
	}
}

export const adminHandlers: RequestHandler[] = [
	http.get(api('/settings'), () => requireSession() ?? HttpResponse.json(settingsRes())),

	http.patch(api('/settings'), async ({ request }) => {
		const denied = requireSession()
		if (denied) return denied
		const body = await readJson<SettingsBody>(request)
		if (!body || typeof body !== 'object') {
			return apiError(400, 'invalid_json', 'el cuerpo no es JSON válido')
		}
		// json/v2 estricto: un campo desconocido es un error.
		if (Object.keys(body).some((key) => !SETTINGS_FIELDS.has(key))) {
			return apiError(400, 'invalid_json', 'campo desconocido')
		}
		const problem = validateSettings(body)
		if (problem) return problem
		if (body.require_2fa === true && !sessionUser()?.totpEnabled) {
			return apiError(
				409,
				'totp_required_first',
				'configura tu propio 2FA antes de exigirlo a todos',
			)
		}
		const changed = applySettings(body)
		// El valor del token de Cloudflare nunca va a la auditoría: solo qué claves cambiaron.
		adminDb.record('settings.update', 'settings', { keys: changed })
		return HttpResponse.json(settingsRes())
	}),

	http.get(api('/audit'), ({ request }) => {
		const denied = requireSession()
		if (denied) return denied
		const params = new URL(request.url).searchParams
		const limitText = params.get('limit')
		const beforeText = params.get('before')
		const limit = limitText === null ? DEFAULT_AUDIT_LIMIT : Number(limitText)
		if (!Number.isInteger(limit) || limit < 1 || limit > MAX_AUDIT_LIMIT) {
			return invalid('limit', `debe estar entre 1 y ${MAX_AUDIT_LIMIT}`)
		}
		const before = beforeText === null ? null : Number(beforeText)
		if (before !== null && (!Number.isInteger(before) || before < 1)) {
			return invalid('before', 'debe ser un id válido')
		}
		const events = [...adminDb.state.audit]
			.filter((event) => before === null || event.id < before)
			.sort((a, b) => b.id - a.id)
			.slice(0, limit)
		return HttpResponse.json(events)
	}),

	// Fase 2: `/registries` (spec §5.1 y §10).

	http.get(api('/registries'), () => {
		const denied = requireSession()
		if (denied) return denied
		return HttpResponse.json(adminDb.state.registries.map(registryRes))
	}),

	http.post(api('/registries'), async ({ request }) => {
		const denied = requireSession()
		if (denied) return denied
		const body = await readJson<RegistryBody>(request)
		if (!body) return apiError(400, 'invalid_json', 'el cuerpo no es JSON válido')
		const host = typeof body.host === 'string' ? body.host : ''
		if (!/^[a-z0-9.-]+(:\d+)?$/.test(host)) return invalid('host', 'no es un host válido')
		if (typeof body.username !== 'string' || body.username === '') {
			return invalid('username', 'es obligatorio')
		}
		if (typeof body.password !== 'string' || body.password === '') {
			return invalid('password', 'es obligatoria')
		}
		if (adminDb.state.registries.some((registry) => registry.host === host)) {
			return apiError(409, 'conflict', 'ya existe')
		}
		const registry = { host, username: body.username, password: body.password }
		adminDb.state.registries = [...adminDb.state.registries, registry]
		adminDb.commit()
		adminDb.record('registry.create', `registry:${host}`)
		return HttpResponse.json(registryRes(registry), { status: 201 })
	}),

	http.patch(api('/registries/:host'), async ({ params, request }) => {
		const denied = requireSession()
		if (denied) return denied
		const host = String(params.host)
		const registry = adminDb.state.registries.find((candidate) => candidate.host === host)
		if (!registry) return notFound()
		const body = await readJson<RegistryBody>(request)
		if (!body) return apiError(400, 'invalid_json', 'el cuerpo no es JSON válido')
		if (typeof body.username === 'string' && body.username !== '') {
			registry.username = body.username
		}
		if (typeof body.password === 'string' && body.password !== '') {
			registry.password = body.password
		}
		adminDb.commit()
		adminDb.record('registry.update', `registry:${host}`)
		return HttpResponse.json(registryRes(registry))
	}),

	http.delete(api('/registries/:host'), ({ params }) => {
		const denied = requireSession()
		if (denied) return denied
		const host = String(params.host)
		if (!adminDb.state.registries.some((registry) => registry.host === host)) return notFound()
		adminDb.state.registries = adminDb.state.registries.filter((r) => r.host !== host)
		adminDb.commit()
		adminDb.record('registry.delete', `registry:${host}`)
		return noContent()
	}),
]
