import { http } from 'msw'
import { afterAll, afterEach, beforeAll, beforeEach, describe, expect, it } from 'vitest'
import { api, apiError } from '@/mocks/api'
import { authDb, MOCK_USERNAME } from '@/mocks/db/auth'
import { ingressDb } from '@/mocks/db/ingress'
import { server } from '@/mocks/server'
import { pendingPhase, UnavailableError, ValidationError } from '@/shared/domain'
import { ingressUseCases } from '../container'
import {
	type AppRef,
	asHostname,
	dnsMode,
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
		expect(manual?.records).toEqual([
			{ type: 'A', name: 'api.staging.otro-dominio.com', content: '203.0.113.24' },
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

		const moved = await ingressUseCases.changeTarget({ app: APP, hostname, target: 'web:8080' })
		expect(moved.target).toEqual({ service: 'web', port: 8080 })

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
