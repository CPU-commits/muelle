import { delay, http, HttpResponse, type RequestHandler } from 'msw'
import { api, apiError, invalid, noContent, notFound } from '../api'
import { adminDb, type MockRegistry } from '../db/admin'
import { authDb } from '../db/auth'
import { requireSession, sessionUser } from '../session'

// `/settings` y `/audit` (admin/infra/in/http), y lo que agregan las Fases 2 y 3 según el
// contrato (docs/superpowers/specs/2026-10-10-contrato-http-fases-2-3.md): los campos de
// proxy y DNS de `/settings` (§4) y `/registries` (§2.10).
//
// Para probar las respuestas de Cloudflare con `pnpm dev:mock`, el token que se pega elige
// la respuesta: con "rechazado" Cloudflare lo rechaza, con "sinzonas" no ve zonas (los dos
// 422 en `cloudflare_token`) y con "caido" no responde (502 `cloudflare_unavailable`).

const MAX_AUDIT_LIMIT = 500
const DEFAULT_AUDIT_LIMIT = 100

const IPV4 = /^((25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)\.){3}(25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)$/
const IPV6 = /^[0-9a-f:.]*:[0-9a-f:.]*$/
const CIDR = /^[0-9a-f:.]+(\/\d{1,3})?$/i
const EMAIL = /^[^\s@]+@[^\s@]+\.[^\s@]{2,}$/
const CLOUDFLARE_TOKEN = /^[\x21-\x7e]{20,512}$/
/** Cuántas zonas "ve" un token que se guarda en el mock. */
const MOCK_CLOUDFLARE_ZONES = 3
/** Lo que el mock tarda en "aplicar" un cambio de puertos (en el navegador). */
const PORT_CHANGE_DELAY_MS = 1500

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
	confirm_lockout?: unknown
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
	'confirm_lockout',
])

function isPort(value: unknown): value is number {
	return typeof value === 'number' && Number.isInteger(value) && value >= 1 && value <= 65535
}

/** `''` borra el valor (contrato §4.1); si no, tiene que cumplir `rule`. */
function clearsOrMatches(value: unknown, rule: RegExp): boolean {
	return value === '' || (typeof value === 'string' && rule.test(value))
}

function ipv4ToNumber(ip: string): number | null {
	if (!IPV4.test(ip)) return null
	return ip.split('.').reduce((acc, part) => acc * 256 + Number(part), 0)
}

// Si `ip` entra en `cidr`. El mock solo sabe de IPv4 (su `client_ip` lo es): una IPv6 de la
// lista nunca la incluye.
function cidrIncludes(cidr: string, ip: string): boolean {
	const [network = '', prefixText] = cidr.split('/')
	const address = ipv4ToNumber(ip)
	const base = ipv4ToNumber(network)
	if (address === null || base === null) return false
	const prefix = prefixText === undefined ? 32 : Number(prefixText)
	const size = 2 ** (32 - prefix)
	return Math.floor(address / size) === Math.floor(base / size)
}

function validateSettings(body: SettingsBody) {
	if (body.require_2fa !== undefined && typeof body.require_2fa !== 'boolean') {
		return apiError(400, 'invalid_json', 'require_2fa debe ser un booleano')
	}
	if (body.confirm_lockout !== undefined && typeof body.confirm_lockout !== 'boolean') {
		return apiError(400, 'invalid_json', 'confirm_lockout debe ser un booleano')
	}
	if (body.public_ip !== undefined && !clearsOrMatches(body.public_ip, IPV4)) {
		return invalid('public_ip', 'no es una IPv4 válida')
	}
	if (body.public_ipv6 !== undefined && !clearsOrMatches(body.public_ipv6, IPV6)) {
		return invalid('public_ipv6', 'no es una IPv6 válida')
	}
	if (body.acme_email !== undefined && !clearsOrMatches(body.acme_email, EMAIL)) {
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
	if (
		body.cloudflare_token !== undefined &&
		!clearsOrMatches(body.cloudflare_token, CLOUDFLARE_TOKEN)
	) {
		return invalid(
			'cloudflare_token',
			'debe tener entre 20 y 512 caracteres visibles, sin espacios',
		)
	}
	return undefined
}

// `CheckCloudflareToken` (contrato §3.5) antes de guardar un token nuevo.
function checkCloudflareToken(token: string) {
	if (token.includes('caido')) {
		return apiError(502, 'cloudflare_unavailable', 'Cloudflare no respondió')
	}
	if (token.includes('rechazado'))
		return invalid('cloudflare_token', 'Cloudflare rechazó el token')
	if (token.includes('sinzonas'))
		return invalid('cloudflare_token', 'el token no ve ninguna zona')
	return undefined
}

function wouldLockOut(body: SettingsBody): boolean {
	const list = body.panel_allowed_cidrs
	if (!Array.isArray(list) || list.length === 0 || body.confirm_lockout === true) return false
	const ip = adminDb.state.clientIp
	return !list.some((cidr) => cidrIncludes(String(cidr), ip))
}

function applySettings(body: SettingsBody): Record<string, unknown> {
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
	if (body.cloudflare_token === '') {
		s.cloudflareToken = null
		s.cloudflareZones = null
	} else if (typeof body.cloudflare_token === 'string') {
		s.cloudflareToken = body.cloudflare_token
		s.cloudflareZones = MOCK_CLOUDFLARE_ZONES
	}
	authDb.commit()
	adminDb.commit()
	// Lo que va a la auditoría: las claves y sus valores, el token solo como "set"/"removed".
	return Object.fromEntries(
		Object.entries(body)
			.filter(([key]) => key !== 'confirm_lockout')
			.map(([key, value]) =>
				key === 'cloudflare_token' ? [key, value === '' ? 'removed' : 'set'] : [key, value],
			),
	)
}

function registryRes(registry: MockRegistry) {
	return {
		host: registry.host,
		username: registry.username,
		created_at: registry.createdAt,
		updated_at: registry.updatedAt,
	}
}

interface RegistryBody {
	host?: unknown
	username?: unknown
	password?: unknown
}

// El host en su forma canónica, sin normalizar (contrato §2.10).
const REGISTRY_LABEL = '[a-z0-9]([a-z0-9-]*[a-z0-9])?'
const REGISTRY_HOST = new RegExp(`^${REGISTRY_LABEL}(\\.${REGISTRY_LABEL})*(:\\d{1,5})?$`)
const DOCKER_HUB_ALIASES = new Set([
	'index.docker.io',
	'registry-1.docker.io',
	'registry.hub.docker.com',
])
const CONTROL = /[\p{Cc}]/u

function validateCredentials(body: RegistryBody) {
	if (typeof body.username !== 'string' || body.username === '' || body.username.length > 255) {
		return invalid('username', 'debe tener entre 1 y 255 caracteres')
	}
	if (CONTROL.test(body.username)) {
		return invalid('username', 'no puede tener caracteres de control')
	}
	if (typeof body.password !== 'string' || body.password === '') {
		return invalid('password', 'es obligatoria')
	}
	if (new TextEncoder().encode(body.password).length > 4096 || /[\r\n]/.test(body.password)) {
		return invalid('password', 'hasta 4096 bytes, sin saltos de línea')
	}
	return undefined
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
		if (wouldLockOut(body)) {
			return apiError(
				409,
				'would_lock_out',
				`con esa lista, tu IP actual (${adminDb.state.clientIp}) no podría abrir el panel`,
			)
		}
		if (typeof body.cloudflare_token === 'string' && body.cloudflare_token !== '') {
			const rejected = checkCloudflareToken(body.cloudflare_token)
			if (rejected) return rejected
		}
		const portsChange =
			body.proxy_http_port !== undefined || body.proxy_https_port !== undefined
		// El hook del proxy corre antes de responder (contrato §3.5): en el navegador se nota.
		if (portsChange && typeof window !== 'undefined') await delay(PORT_CHANGE_DELAY_MS)
		adminDb.record('settings.update', 'settings', applySettings(body))
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

	// Fase 2: `/registries` (contrato §2.10).

	http.get(api('/registries'), () => {
		const denied = requireSession()
		if (denied) return denied
		const sorted = [...adminDb.state.registries].sort((a, b) => a.host.localeCompare(b.host))
		return HttpResponse.json(sorted.map(registryRes))
	}),

	http.post(api('/registries'), async ({ request }) => {
		const denied = requireSession()
		if (denied) return denied
		const body = await readJson<RegistryBody>(request)
		if (!body) return apiError(400, 'invalid_json', 'el cuerpo no es JSON válido')
		const host = typeof body.host === 'string' ? body.host : ''
		if (DOCKER_HUB_ALIASES.has(host)) return invalid('host', 'para Docker Hub usa docker.io')
		if (!REGISTRY_HOST.test(host)) return invalid('host', 'no es un host en su forma canónica')
		const problem = validateCredentials(body)
		if (problem) return problem
		if (adminDb.state.registries.some((registry) => registry.host === host)) {
			return apiError(409, 'conflict', 'ya existe')
		}
		const now = new Date().toISOString()
		const registry: MockRegistry = {
			host,
			username: String(body.username),
			password: String(body.password),
			createdAt: now,
			updatedAt: now,
		}
		adminDb.state.registries = [...adminDb.state.registries, registry]
		adminDb.commit()
		adminDb.record('registry.create', `registry:${host}`, { username: registry.username })
		return HttpResponse.json(registryRes(registry), { status: 201 })
	}),

	// Reemplaza las credenciales: usuario y contraseña, los dos.
	http.patch(api('/registries/:host'), async ({ params, request }) => {
		const denied = requireSession()
		if (denied) return denied
		const host = decodeURIComponent(String(params.host))
		const registry = adminDb.state.registries.find((candidate) => candidate.host === host)
		if (!registry) return notFound()
		const body = await readJson<RegistryBody>(request)
		if (!body) return apiError(400, 'invalid_json', 'el cuerpo no es JSON válido')
		const problem = validateCredentials(body)
		if (problem) return problem
		registry.username = String(body.username)
		registry.password = String(body.password)
		registry.updatedAt = new Date().toISOString()
		adminDb.commit()
		adminDb.record('registry.update', `registry:${host}`, { username: registry.username })
		return HttpResponse.json(registryRes(registry))
	}),

	http.delete(api('/registries/:host'), ({ params }) => {
		const denied = requireSession()
		if (denied) return denied
		const host = decodeURIComponent(String(params.host))
		if (!adminDb.state.registries.some((registry) => registry.host === host)) return notFound()
		adminDb.state.registries = adminDb.state.registries.filter((r) => r.host !== host)
		adminDb.commit()
		adminDb.record('registry.delete', `registry:${host}`)
		return noContent()
	}),
]
