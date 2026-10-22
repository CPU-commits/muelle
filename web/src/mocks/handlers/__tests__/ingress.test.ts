import { afterAll, beforeAll, beforeEach, describe, expect, it } from 'vitest'
import { api } from '@/mocks/api'
import { adminDb } from '@/mocks/db/admin'
import { authDb, MOCK_USERNAME } from '@/mocks/db/auth'
import { ingressDb } from '@/mocks/db/ingress'
import { server } from '@/mocks/server'

// `cert_source` y `origin_ca` de los dominios y del panel en el mock (contrato §3.1, §3.3 y
// §3.7), calculados con los ajustes de `db/admin.ts` y las reglas de `db/ingress.ts`.
beforeAll(() => server.listen({ onUnhandledRequest: 'error' }))
beforeEach(() => {
	authDb.reset({ session: { username: MOCK_USERNAME, mfaPending: false, badCodes: 0 } })
	adminDb.reset()
	// Sin la espera de la primera pasada: los certificados de Cloudflare salen en el acto.
	ingressDb.reset({ provisionMs: 0 })
})
afterAll(() => server.close())

const DOMAINS = api('/projects/auditfy/stages/staging/apps/api/domains')

interface DomainJson {
	hostname: string
	proxied: boolean
	cert_source: string
	origin_ca: {
		status: string
		error_code: string | null
		error: string | null
		retry_at: string | null
		expires_at: string | null
	} | null
}

async function json<T>(url: string, init?: RequestInit): Promise<T> {
	return (await (await fetch(url, init)).json()) as T
}

const listDomains = () => json<DomainJson[]>(DOMAINS)
const findDomain = async (hostname: string) =>
	(await listDomains()).find((domain) => domain.hostname === hostname)

async function addDomain(hostname: string, proxied = true): Promise<DomainJson> {
	return json<DomainJson>(DOMAINS, {
		method: 'POST',
		headers: { 'content-type': 'application/json' },
		body: JSON.stringify({ hostname, target_service: 'api', target_port: 3000, proxied }),
	})
}

const turnOn = () =>
	fetch(api('/settings'), {
		method: 'PATCH',
		headers: { 'content-type': 'application/json' },
		body: JSON.stringify({ cloudflare_origin_ca: true }),
	})

const NO_FAILURE = { error_code: null, error: null, retry_at: null }

describe('origin_ca de los dominios', () => {
	it("con la opción apagada todo es Let's Encrypt y origin_ca es null", async () => {
		for (const domain of await listDomains()) {
			expect(domain).toMatchObject({ cert_source: 'letsencrypt', origin_ca: null })
		}
	})

	it('encendida: el manejado con proxy se sirve con Origin CA (90 días); el ajeno y el manual no califican', async () => {
		await listDomains()
		await turnOn()

		const domains = Object.fromEntries((await listDomains()).map((d) => [d.hostname, d]))
		const active = domains['api-staging.tudominio.cl']
		expect(active).toMatchObject({
			cert_source: 'cloudflare_origin_ca',
			origin_ca: { status: 'active', ...NO_FAILURE },
		})
		const expiresInDays =
			(Date.parse(active?.origin_ca?.expires_at ?? '') - Date.now()) / (24 * 60 * 60 * 1000)
		expect(Math.round(expiresInDays)).toBe(90)
		for (const hostname of ['webhooks-staging.tudominio.cl', 'api.staging.otro-dominio.com']) {
			expect(domains[hostname]).toMatchObject({
				cert_source: 'letsencrypt',
				origin_ca: { status: 'not_eligible', ...NO_FAILURE, expires_at: null },
			})
		}
	})

	it("mientras corre la primera pasada dicen issuing, con Let's Encrypt", async () => {
		ingressDb.reset({ provisionMs: 60_000 })
		await turnOn()

		expect(await findDomain('api-staging.tudominio.cl')).toMatchObject({
			cert_source: 'letsencrypt',
			origin_ca: { status: 'issuing', ...NO_FAILURE, expires_at: null },
		})
	})

	it('uno manejado sin Proxied es dns_only; con gris- lo apagaron en Cloudflare (Proxied sigue en muelle)', async () => {
		await turnOn()
		await addDomain('directo.tudominio.cl', false)
		await addDomain('gris-panel.tudominio.cl', true)

		expect(await findDomain('directo.tudominio.cl')).toMatchObject({
			proxied: false,
			cert_source: 'letsencrypt',
			origin_ca: { status: 'dns_only', ...NO_FAILURE, expires_at: null },
		})
		expect(await findDomain('gris-panel.tudominio.cl')).toMatchObject({
			proxied: true,
			cert_source: 'letsencrypt',
			origin_ca: { status: 'dns_only' },
		})
	})

	it("con falla- la emisión falla por el permiso: Let's Encrypt mientras tanto, con el mensaje del backend", async () => {
		await turnOn()
		await addDomain('falla-ssl.tudominio.cl')

		const domain = await findDomain('falla-ssl.tudominio.cl')
		expect(domain).toMatchObject({
			cert_source: 'letsencrypt',
			origin_ca: {
				status: 'failed',
				error_code: 'permission',
				error: "muelle no pudo emitir el certificado de Cloudflare: el token no tiene el permiso Zone → SSL and Certificates → Edit en la zona de falla-ssl.tudominio.cl (Cloudflare: Authentication error); agrégalo al token y pulsa Reintentar; mientras tanto usa Let's Encrypt",
				expires_at: null,
			},
		})
		// Se reintenta solo en 6 h.
		const retryIn = Date.parse(domain?.origin_ca?.retry_at ?? '') - Date.now()
		expect(Math.round(retryIn / (60 * 60 * 1000))).toBe(6)
	})

	it('Reintentar reinicia la espera: la respuesta trae retry_at en la hora del pedido', async () => {
		ingressDb.reset({ provisionMs: 60_000, originCaSince: Date.now() - 120_000 })
		adminDb.state.cloudflareOriginCa = true
		await addDomain('falla-ssl.tudominio.cl')
		const before = Date.now()

		const retried = await json<DomainJson>(`${DOMAINS}/falla-ssl.tudominio.cl/retry`, {
			method: 'POST',
		})

		expect(retried.origin_ca).toMatchObject({ status: 'failed', error_code: 'permission' })
		const retryAt = Date.parse(retried.origin_ca?.retry_at ?? '')
		expect(retryAt).toBeGreaterThanOrEqual(before - 1000)
		expect(retryAt).toBeLessThanOrEqual(Date.now())
	})

	it('con renueva- falló la renovación: sigue con Origin CA, con el aviso y su vencimiento', async () => {
		await turnOn()
		await addDomain('renueva-api.tudominio.cl')

		const domain = await findDomain('renueva-api.tudominio.cl')
		expect(domain).toMatchObject({
			cert_source: 'cloudflare_origin_ca',
			origin_ca: { status: 'active', error_code: 'cloudflare' },
		})
		expect(domain?.origin_ca?.error).toMatch(
			/^muelle no pudo renovar el certificado de Cloudflare, que vence el \d{1,2} [a-z]{3} \d{4}: Cloudflare no respondió \(HTTP 503\); se reintenta solo; si no lo renueva antes del \d{1,2} [a-z]{3} \d{4}, vuelve a Let's Encrypt$/,
		)
		expect(domain?.origin_ca?.retry_at).not.toBeNull()
		expect(domain?.origin_ca?.expires_at).not.toBeNull()
	})

	it('sin token, o sin el proxy de muelle (sin dominio del panel), origin_ca es null', async () => {
		await turnOn()
		adminDb.state.panelDomain = ''
		expect(await findDomain('api-staging.tudominio.cl')).toMatchObject({
			cert_source: 'letsencrypt',
			origin_ca: null,
		})
		adminDb.state.panelDomain = 'panel.tudominio.cl'
		adminDb.state.cloudflareToken = null
		expect(await findDomain('api-staging.tudominio.cl')).toMatchObject({ origin_ca: null })
	})
})

describe('panel_certificate de GET /proxy', () => {
	interface ProxyJson {
		panel_certificate: Omit<DomainJson, 'proxied'> | null
	}
	const proxy = () => json<ProxyJson>(api('/proxy'))

	it('encendida, el panel (en la zona) se sirve con Origin CA', async () => {
		await turnOn()

		expect((await proxy()).panel_certificate).toMatchObject({
			hostname: 'panel.tudominio.cl',
			cert_source: 'cloudflare_origin_ca',
			origin_ca: { status: 'active', ...NO_FAILURE },
		})
	})

	it('su falla dice que se reintenta solo: el panel no tiene Reintentar', async () => {
		adminDb.state.panelDomain = 'falla-panel.tudominio.cl'
		await turnOn()

		expect((await proxy()).panel_certificate).toMatchObject({
			hostname: 'falla-panel.tudominio.cl',
			cert_source: 'letsencrypt',
			origin_ca: {
				status: 'failed',
				error_code: 'permission',
				error: "muelle no pudo emitir el certificado de Cloudflare: el token no tiene el permiso Zone → SSL and Certificates → Edit en la zona de falla-panel.tudominio.cl (Cloudflare: Authentication error); agrégalo al token: muelle lo reintenta solo cada 6 h; mientras tanto usa Let's Encrypt",
			},
		})
	})

	it("en DNS only en Cloudflare (gris-), o fuera de la zona, Let's Encrypt", async () => {
		await turnOn()
		adminDb.state.panelDomain = 'gris-panel.tudominio.cl'
		expect((await proxy()).panel_certificate?.origin_ca?.status).toBe('dns_only')
		adminDb.state.panelDomain = 'panel.otro-dominio.com'
		expect((await proxy()).panel_certificate?.origin_ca?.status).toBe('not_eligible')
	})

	it("apagada, Let's Encrypt y sin origin_ca", async () => {
		expect((await proxy()).panel_certificate).toEqual({
			hostname: 'panel.tudominio.cl',
			cert_source: 'letsencrypt',
			origin_ca: null,
		})
	})
})
