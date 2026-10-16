import { http } from 'msw'
import { afterAll, afterEach, beforeAll, beforeEach, describe, expect, it } from 'vitest'
import { api, apiError } from '@/mocks/api'
import { adminDb } from '@/mocks/db/admin'
import { authDb, MOCK_USERNAME } from '@/mocks/db/auth'
import { ingressDb } from '@/mocks/db/ingress'
import { server } from '@/mocks/server'
import { pendingPhase, UnavailableError, ValidationError } from '@/shared/domain'
import { ingressUseCases } from '../container'
import {
	type AppRef,
	asHostname,
	CloudflareUnavailable,
	dnsMode,
	DockerUnavailable,
	DomainNotFound,
	domainState,
	HostnameTaken,
	NothingToAdopt,
} from '../domain'

// Recorre el módulo entero —caso de uso, repositorio, datasource, schema Zod y mapper—
// contra el backend simulado de src/mocks/handlers/ingress.ts.
const APP: AppRef = { project: 'auditfy', stage: 'staging', app: 'api' }
const DOMAINS = '/projects/:p/stages/:s/apps/:a/domains'

beforeAll(() => server.listen({ onUnhandledRequest: 'error' }))
beforeEach(() => {
	authDb.reset({ session: { username: MOCK_USERNAME, mfaPending: false, badCodes: 0 } })
	// Los certificados se "emiten" en 30 ms en vez de 3 s.
	ingressDb.reset({ provisionMs: 30 })
	// El token de Cloudflare y las IP públicas salen de los ajustes del mock (contrato §3.1).
	adminDb.reset()
})
afterEach(() => server.resetHandlers())
afterAll(() => server.close())

const wait = (ms: number) => new Promise((resolve) => setTimeout(resolve, ms))

describe('dominios de una app', () => {
	it('la siembra del diseño: válido, registro existente y esperando DNS', async () => {
		const domains = await ingressUseCases.listDomains(APP)

		expect(domains.map((domain) => [domain.hostname, domainState(domain)])).toEqual([
			['api-staging.tudominio.cl', 'active'],
			['api.staging.otro-dominio.com', 'waiting_dns'],
			['webhooks-staging.tudominio.cl', 'record_exists'],
		])
		const [active, manual] = domains
		expect(active?.certificate?.issuer).toBe("Let's Encrypt")
		expect(dnsMode(manual!)).toBe('manual')
		// El A y el AAAA (el nodo tiene IPv6): los dos se crean a mano.
		expect(manual?.records).toEqual([
			{ type: 'A', name: 'api.staging.otro-dominio.com', content: '203.0.113.24' },
			{ type: 'AAAA', name: 'api.staging.otro-dominio.com', content: '2001:db8::24' },
		])
	})

	it('agregar en la zona: pending y a los segundos active con certificado', async () => {
		const created = await ingressUseCases.addDomain({
			app: APP,
			hostname: 'Nuevo.TuDominio.cl',
			target: 'api:3000',
			proxied: true,
		})

		expect(created.hostname).toBe('nuevo.tudominio.cl')
		expect(domainState(created)).toBe('provisioning')
		expect(dnsMode(created)).toBe('cloudflare')

		await wait(60)
		const later = (await ingressUseCases.listDomains(APP)).find(
			(domain) => domain.hostname === 'nuevo.tudominio.cl',
		)
		expect(later?.status).toBe('active')
		expect(later?.certificate).not.toBeNull()
	})

	it('un hostname que ya existe en muelle es HostnameTaken', async () => {
		await ingressUseCases.listDomains(APP)
		await expect(
			ingressUseCases.addDomain({
				app: APP,
				hostname: 'api-staging.tudominio.cl',
				target: 'api:3000',
				proxied: true,
			}),
		).rejects.toBeInstanceOf(HostnameTaken)
	})

	it('un 422 del backend sale como ValidationError del campo del panel', async () => {
		server.use(
			http.post(api(DOMAINS), () =>
				apiError(422, 'invalid', 'target_service: el servicio no existe en la app'),
			),
		)
		const result = ingressUseCases.addDomain({
			app: APP,
			hostname: 'otro.tudominio.cl',
			target: 'worker:9000',
			proxied: false,
		})
		await expect(result).rejects.toBeInstanceOf(ValidationError)
		await expect(result).rejects.toMatchObject({
			field: 'target',
			message: 'el servicio no existe en la app',
		})
	})

	it('"el registro ya existe" → Adoptar → muelle lo maneja y queda activo', async () => {
		const hostname = asHostname('webhooks-staging.tudominio.cl')
		await ingressUseCases.listDomains(APP)

		const adopted = await ingressUseCases.adoptRecord({ app: APP, hostname })
		expect(adopted.recordId).not.toBeNull()
		expect(domainState(adopted)).toBe('provisioning')

		await wait(60)
		const later = (await ingressUseCases.listDomains(APP)).find(
			(domain) => domain.hostname === hostname,
		)
		expect(domainState(later!)).toBe('active')
	})

	it('adoptar uno que no tiene registro existente es NothingToAdopt', async () => {
		await ingressUseCases.listDomains(APP)
		await expect(
			ingressUseCases.adoptRecord({
				app: APP,
				hostname: asHostname('api-staging.tudominio.cl'),
			}),
		).rejects.toBeInstanceOf(NothingToAdopt)
	})

	it('un error de Cloudflare se reintenta', async () => {
		const created = await ingressUseCases.addDomain({
			app: APP,
			hostname: 'cf-error.tudominio.cl',
			target: 'api:3000',
			proxied: true,
		})
		expect(domainState(created)).toBe('error')
		expect(created.errorKind).toBe('cloudflare')

		const retried = await ingressUseCases.retryDomain({ app: APP, hostname: created.hostname })
		expect(domainState(retried)).toBe('provisioning')
	})

	it('Verificar un dominio manual lo activa cuando el DNS ya resuelve', async () => {
		await ingressUseCases.listDomains(APP)
		const verified = await ingressUseCases.retryDomain({
			app: APP,
			hostname: asHostname('api.staging.otro-dominio.com'),
		})
		expect(domainState(verified)).toBe('active')
	})

	it('Proxied y destino se editan; Proxied sin registro de muelle es un 422', async () => {
		await ingressUseCases.listDomains(APP)
		const hostname = asHostname('api-staging.tudominio.cl')

		const off = await ingressUseCases.setProxied({ app: APP, hostname, proxied: false })
		expect(off.proxied).toBe(false)

		const moved = await ingressUseCases.changeTarget({ app: APP, hostname, target: 'api:8080' })
		expect(moved.target).toEqual({ service: 'api', port: 8080 })

		// En una app de formulario el destino es su único servicio, el de la app.
		await expect(
			ingressUseCases.changeTarget({ app: APP, hostname, target: 'web:8080' }),
		).rejects.toMatchObject({ name: 'ValidationError', field: 'target' })

		await expect(
			ingressUseCases.setProxied({
				app: APP,
				hostname: asHostname('api.staging.otro-dominio.com'),
				proxied: true,
			}),
		).rejects.toMatchObject({ name: 'ValidationError', field: 'proxied' })
	})

	it('borrar lo saca de la lista; uno que no existe es DomainNotFound (no la fase)', async () => {
		await ingressUseCases.listDomains(APP)
		const hostname = asHostname('api-staging.tudominio.cl')

		await ingressUseCases.removeDomain({ app: APP, hostname })
		const domains = await ingressUseCases.listDomains(APP)
		expect(domains.some((domain) => domain.hostname === hostname)).toBe(false)

		const missing = ingressUseCases.removeDomain({ app: APP, hostname })
		await expect(missing).rejects.toBeInstanceOf(DomainNotFound)
		await expect(missing).rejects.toSatisfy((error) => pendingPhase(error) === null)
	})
})

describe('contra un backend sin la Fase 3', () => {
	it('una ruta que no existe (no_route) es UnavailableError con phase 3', async () => {
		server.use(http.get(api(DOMAINS), () => apiError(404, 'no_route', 'no existe la ruta')))
		const result = ingressUseCases.listDomains(APP)
		await expect(result).rejects.toBeInstanceOf(UnavailableError)
		await expect(result).rejects.toSatisfy((error) => pendingPhase(error) === 3)
	})

	it('un método que no existe (405) también, en cualquier operación', async () => {
		server.use(
			http.post(api(DOMAINS), () =>
				apiError(405, 'method_not_allowed', 'método no permitido'),
			),
			http.post(api(`${DOMAINS}/:hostname/adopt`), () =>
				apiError(404, 'no_route', 'no existe la ruta'),
			),
		)
		await expect(
			ingressUseCases.addDomain({
				app: APP,
				hostname: 'x.tudominio.cl',
				target: 'api:3000',
				proxied: false,
			}),
		).rejects.toSatisfy((error) => pendingPhase(error) === 3)
		await expect(
			ingressUseCases.adoptRecord({ app: APP, hostname: asHostname('x.tudominio.cl') }),
		).rejects.toSatisfy((error) => pendingPhase(error) === 3)
	})
})

describe('contrato de la Fase 3', () => {
	it('adoptar conserva el Proxied del registro ajeno y lo copia al dominio', async () => {
		const hostname = asHostname('webhooks-staging.tudominio.cl')
		const before = (await ingressUseCases.listDomains(APP)).find(
			(domain) => domain.hostname === hostname,
		)
		expect(before?.proxied).toBe(false)

		const adopted = await ingressUseCases.adoptRecord({ app: APP, hostname })
		expect(adopted.proxied).toBe(true)
		expect(adopted.recordId).not.toBeNull()
		expect(adopted.status).toBe('pending')
	})

	it('reintentar un "ya existe": si el registro ajeno sigue, sigue igual', async () => {
		const created = await ingressUseCases.addDomain({
			app: APP,
			hostname: 'existe.tudominio.cl',
			target: 'api:3000',
			proxied: false,
		})
		expect(domainState(created)).toBe('record_exists')

		const again = await ingressUseCases.retryDomain({ app: APP, hostname: created.hostname })
		expect(domainState(again)).toBe('record_exists')
	})

	it('reintentar un "ya existe" cuyo registro ajeno ya no está lo crea', async () => {
		const created = await ingressUseCases.addDomain({
			app: APP,
			hostname: 'existe-tmp.tudominio.cl',
			target: 'api:3000',
			proxied: false,
		})
		// El registro ajeno se borró en Cloudflare.
		const stored = ingressDb.findAnywhere(created.hostname)
		if (!stored) throw new Error('sin dominio')
		ingressDb.save({ ...stored, foreignRecord: { proxied: true, goneAt: 0 } })

		const retried = await ingressUseCases.retryDomain({ app: APP, hostname: created.hostname })
		expect(retried.status).toBe('pending')
		expect(retried.recordId).not.toBeNull()
	})

	it('borrar con Cloudflare caído es CloudflareUnavailable y no borra nada', async () => {
		const created = await ingressUseCases.addDomain({
			app: APP,
			hostname: 'cf-caido.tudominio.cl',
			target: 'api:3000',
			proxied: false,
		})
		await expect(
			ingressUseCases.removeDomain({ app: APP, hostname: created.hostname }),
		).rejects.toBeInstanceOf(CloudflareUnavailable)
		const domains = await ingressUseCases.listDomains(APP)
		expect(domains.some((domain) => domain.hostname === created.hostname)).toBe(true)
	})

	it('Docker caído (503 docker_unavailable) es DockerUnavailable', async () => {
		server.use(
			http.get(api(DOMAINS), () => apiError(503, 'docker_unavailable', 'Docker no responde')),
		)
		await expect(ingressUseCases.listDomains(APP)).rejects.toBeInstanceOf(DockerUnavailable)
	})

	it('cada dominio trae updated_at, que cambia con cada cambio', async () => {
		const [first] = await ingressUseCases.listDomains(APP)
		if (!first) throw new Error('sin dominios')
		expect(first.updatedAt).toBeInstanceOf(Date)
		await wait(5)
		const changed = await ingressUseCases.setProxied({
			app: APP,
			hostname: first.hostname,
			proxied: !first.proxied,
		})
		expect(changed.updatedAt?.getTime()).toBeGreaterThan(first.updatedAt?.getTime() ?? 0)
	})

	it('Cloudflare que falla al cambiar Proxied o al adoptar: 200 con el dominio en error', async () => {
		const created = await ingressUseCases.addDomain({
			app: APP,
			hostname: 'cf-intermitente.tudominio.cl',
			target: 'api:3000',
			proxied: false,
		})
		const toggled = await ingressUseCases.setProxied({
			app: APP,
			hostname: created.hostname,
			proxied: true,
		})
		// Se guardó lo pedido, y el dominio quedó en error para reintentar.
		expect(toggled.proxied).toBe(true)
		expect(domainState(toggled)).toBe('error')
		expect(toggled.errorKind).toBe('cloudflare')

		const existing = await ingressUseCases.addDomain({
			app: APP,
			hostname: 'existe-cf-intermitente.tudominio.cl',
			target: 'api:3000',
			proxied: false,
		})
		const adopted = await ingressUseCases.adoptRecord({ app: APP, hostname: existing.hostname })
		expect(adopted.errorKind).toBe('cloudflare')
	})
})

describe('el mock sigue a los ajustes, como el backend', () => {
	it('records se calcula con las IP públicas de ahora: sin IPv6 no hay AAAA, y sin IP nada', async () => {
		adminDb.state.publicIpv6 = ''
		const [, manual] = await ingressUseCases.listDomains(APP)
		expect(manual?.records).toEqual([
			{ type: 'A', name: 'api.staging.otro-dominio.com', content: '203.0.113.24' },
		])

		adminDb.state.publicIp = '198.51.100.9'
		const [, moved] = await ingressUseCases.listDomains(APP)
		expect(moved?.records[0]?.content).toBe('198.51.100.9')

		adminDb.state.publicIp = ''
		const [, none] = await ingressUseCases.listDomains(APP)
		expect(none?.records).toEqual([])
	})

	it('solo con IPv6, records trae el AAAA solo (entity.Records arma cada uno por su lado)', async () => {
		adminDb.state.publicIp = ''
		const [, manual] = await ingressUseCases.listDomains(APP)
		expect(manual?.records).toEqual([
			{ type: 'AAAA', name: 'api.staging.otro-dominio.com', content: '2001:db8::24' },
		])
	})

	it('sin token de Cloudflare, un hostname de la zona queda manual y sin Proxied', async () => {
		adminDb.state.cloudflareToken = null
		const created = await ingressUseCases.addDomain({
			app: APP,
			hostname: 'sin-token.tudominio.cl',
			target: 'api:3000',
			proxied: true,
		})
		expect(dnsMode(created)).toBe('manual')
		expect(created.proxied).toBe(false)
		expect(domainState(created)).toBe('waiting_dns')
	})

	it('Proxied que sale bien saca un error de Cloudflare: vuelve a pending', async () => {
		const created = await ingressUseCases.addDomain({
			app: APP,
			hostname: 'arregla.tudominio.cl',
			target: 'api:3000',
			proxied: false,
		})
		ingressDb.save({
			...ingressDb.find('auditfy/staging/api', created.hostname)!,
			status: 'error',
			lastError: 'Cloudflare respondió 500',
			lastErrorCode: 'cloudflare',
			activeAt: null,
		})
		const toggled = await ingressUseCases.setProxied({
			app: APP,
			hostname: created.hostname,
			proxied: true,
		})
		expect(toggled.status).toBe('pending')
		expect(toggled.errorKind).toBeNull()
	})

	it('Proxied sin token (se quitó en Ajustes): se guarda y el dominio queda en error de Cloudflare (200)', async () => {
		// Un dominio cuyo registro maneja muelle (con cf_record_id).
		const created = await ingressUseCases.addDomain({
			app: APP,
			hostname: 'sin-token-despues.tudominio.cl',
			target: 'api:3000',
			proxied: false,
		})
		adminDb.state.cloudflareToken = null

		const toggled = await ingressUseCases.setProxied({
			app: APP,
			hostname: created.hostname,
			proxied: true,
		})
		expect(toggled.proxied).toBe(true)
		expect(domainState(toggled)).toBe('error')
		expect(toggled.errorKind).toBe('cloudflare')
		expect(toggled.lastError).toBe('agrega el token de Cloudflare en Ajustes')
	})

	it('adoptar sin IP pública deja el dominio en error de Cloudflare, con qué falta (200)', async () => {
		await ingressUseCases.listDomains(APP)
		adminDb.state.publicIp = ''
		const adopted = await ingressUseCases.adoptRecord({
			app: APP,
			hostname: asHostname('webhooks-staging.tudominio.cl'),
		})
		expect(adopted.errorKind).toBe('cloudflare')
		expect(adopted.lastError).toMatch(/IP pública/)
	})
})

describe('GET /proxy', () => {
	it('sin dominio del panel está apagado, sin contenedor (image null) y sin error', async () => {
		adminDb.state.panelDomain = ''
		const proxy = await ingressUseCases.getProxyStatus()
		expect(proxy.status).toBe('disabled')
		expect(proxy.caddy).toMatchObject({ image: null, running: false })
		expect(proxy.error).toBeNull()
	})

	it('sincronizado, con muelle-caddy corriendo', async () => {
		const proxy = await ingressUseCases.getProxyStatus()
		expect(proxy.status).toBe('synced')
		expect(proxy.caddy).toMatchObject({ running: true, httpPort: 80, httpsPort: 443 })
		expect(proxy.syncedAt).toBeInstanceOf(Date)
	})

	it('un dominio que Caddy rechaza lo deja desincronizado, con el motivo', async () => {
		await ingressUseCases.addDomain({
			app: APP,
			hostname: 'caddy-rechaza.tudominio.cl',
			target: 'api:3000',
			proxied: false,
		})
		const proxy = await ingressUseCases.getProxyStatus()
		expect(proxy.status).toBe('out_of_sync')
		expect(proxy.error).toMatch(/Caddy rechazó/)
	})

	it('sin la Fase 3 (no_route) es la fase: la pantalla no muestra nada', async () => {
		server.use(http.get(api('/proxy'), () => apiError(404, 'no_route', 'no existe')))
		await expect(ingressUseCases.getProxyStatus()).rejects.toSatisfy(
			(error) => pendingPhase(error) === 3,
		)
	})
})
