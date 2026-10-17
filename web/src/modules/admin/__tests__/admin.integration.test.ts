import { http, HttpResponse } from 'msw'
import { afterAll, afterEach, beforeAll, beforeEach, describe, expect, it } from 'vitest'
import { api, apiError, invalid, noRoute } from '@/mocks/api'
import { adminDb, MOCK_AUDIT_SEED_COUNT } from '@/mocks/db/admin'
import { authDb, MOCK_USERNAME } from '@/mocks/db/auth'
import { identityDb } from '@/mocks/db/identity'
import { ingressDb } from '@/mocks/db/ingress'
import { server } from '@/mocks/server'
import { pendingPhase, UnavailableError, ValidationError } from '@/shared/domain'
import { ingressUseCases } from '@/modules/ingress'
import { adminUseCases } from '../container'
import {
	AUDIT_PAGE_SIZE,
	CloudflareUnavailable,
	InvalidCidr,
	RegistryExists,
	RegistryNotFound,
	TotpRequiredFirst,
	WouldLockOut,
} from '../domain'

// Recorre el módulo entero contra el backend simulado (src/mocks/handlers/admin.ts). Los
// mocks responden como el backend con las Fases 2 y 3; con `server.use` se simula el de la
// Fase 1, que todavía no las tiene.
beforeAll(() => server.listen({ onUnhandledRequest: 'error' }))
beforeEach(() => {
	authDb.reset()
	identityDb.reset()
	adminDb.reset()
	ingressDb.reset()
	authDb.state.session = { username: MOCK_USERNAME, mfaPending: false, badCodes: 0 }
	authDb.commit()
})
afterEach(() => server.resetHandlers())
afterAll(() => server.close())

function enableOwnTotp(): void {
	const user = authDb.findUser(MOCK_USERNAME)
	if (user) user.totpEnabled = true
}

const phaseOneSettings = () =>
	http.get(api('/settings'), () => HttpResponse.json({ require_2fa: false }))

describe('settings', () => {
	it('con la Fase 3, trae proxy, DNS, certificados e IPs permitidas', async () => {
		await expect(adminUseCases.getSettings()).resolves.toEqual({
			require2fa: false,
			proxy: {
				cloudflare: { tokenSet: true, zones: 4 },
				publicIpv4: '203.0.113.24',
				publicIpv6: '2001:db8::24',
				acmeEmail: 'ops@tudominio.cl',
				panelDomain: 'panel.tudominio.cl',
				httpPort: 80,
				httpsPort: 443,
			},
			panelAccess: { allowedCidrs: [], clientIp: '198.51.100.17' },
		})
	})

	it('el backend de la Fase 1 solo tiene require_2fa: lo demás queda pendiente', async () => {
		server.use(phaseOneSettings())

		await expect(adminUseCases.getSettings()).resolves.toEqual({
			require2fa: false,
			proxy: null,
			panelAccess: null,
		})
	})

	it('exigir 2FA sin tener la propia es TotpRequiredFirst; con ella, se activa', async () => {
		await expect(adminUseCases.updateSecurity({ require2fa: true })).rejects.toBeInstanceOf(
			TotpRequiredFirst,
		)
		expect(authDb.state.require2fa).toBe(false)

		enableOwnTotp()
		await expect(adminUseCases.updateSecurity({ require2fa: true })).resolves.toMatchObject({
			require2fa: true,
		})
		expect(authDb.state.require2fa).toBe(true)
	})

	it('en la Fase 1 solo se manda require_2fa', async () => {
		enableOwnTotp()
		let sent: unknown
		server.use(
			http.patch(api('/settings'), async ({ request }) => {
				sent = await request.json()
				return HttpResponse.json({ require_2fa: true })
			}),
		)

		await adminUseCases.updateSecurity({ require2fa: true })

		expect(sent).toEqual({ require_2fa: true })
	})

	it('una lista que deja fuera la IP de la sesión es WouldLockOut; con confirmLockout se guarda', async () => {
		// El mock ve al navegador como 198.51.100.17.
		await expect(
			adminUseCases.updateSecurity({ allowedCidrs: '10.0.0.0/8' }),
		).rejects.toBeInstanceOf(WouldLockOut)
		expect(adminDb.state.panelAllowedCidrs).toEqual([])

		await expect(
			adminUseCases.updateSecurity({ allowedCidrs: '10.0.0.0/8', confirmLockout: true }),
		).resolves.toMatchObject({ panelAccess: { allowedCidrs: ['10.0.0.0/8'] } })
		// confirm_lockout no es un ajuste: no queda en la auditoría.
		expect(adminDb.state.audit.at(-1)?.details).toEqual({
			panel_allowed_cidrs: ['10.0.0.0/8'],
		})
	})

	it('confirm_lockout solo va cuando se confirma', async () => {
		const sent: unknown[] = []
		server.use(
			http.patch(api('/settings'), async ({ request }) => {
				sent.push(await request.json())
				return apiError(409, 'would_lock_out', 'te quedarías fuera')
			}),
		)

		await adminUseCases.updateSecurity({ allowedCidrs: '10.0.0.0/8' }).catch(() => null)
		await adminUseCases
			.updateSecurity({ allowedCidrs: '10.0.0.0/8', confirmLockout: true })
			.catch(() => null)

		expect(sent).toEqual([
			{ panel_allowed_cidrs: ['10.0.0.0/8'] },
			{ panel_allowed_cidrs: ['10.0.0.0/8'], confirm_lockout: true },
		])
	})

	it('client_ip siempre es un string', async () => {
		const settings = await adminUseCases.getSettings()

		expect(settings.panelAccess?.clientIp).toBe('198.51.100.17')
	})

	it('guarda las IPs permitidas, validadas antes de mandarlas', async () => {
		await expect(
			adminUseCases.updateSecurity({ allowedCidrs: '198.51.100.0/24\n203.0.113.7\n' }),
		).resolves.toMatchObject({
			panelAccess: { allowedCidrs: ['198.51.100.0/24', '203.0.113.7'] },
		})
		await expect(
			adminUseCases.updateSecurity({ allowedCidrs: '198.51.100.0/24\nmal' }),
		).rejects.toBeInstanceOf(InvalidCidr)
	})

	it('un 422 de un campo del backend llega con el nombre del formulario', async () => {
		server.use(http.patch(api('/settings'), () => invalid('public_ip', 'no es pública')))

		const error = await adminUseCases
			.updatePublicIp({ ipv4: '10.0.0.1', ipv6: '' })
			.catch((caught: unknown) => caught)

		expect(error).toBeInstanceOf(ValidationError)
		expect((error as ValidationError).field).toBe('ipv4')
		expect((error as ValidationError).message).toBe('no es pública')
	})

	it('IP pública, email ACME y puertos', async () => {
		await adminUseCases.updatePublicIp({ ipv4: '203.0.113.30', ipv6: '' })
		await adminUseCases.updateAcmeEmail('certs@tudominio.cl')
		const settings = await adminUseCases.updateProxyPorts({ http: '8080', https: '8443' })

		expect(settings.proxy).toMatchObject({
			publicIpv4: '203.0.113.30',
			publicIpv6: '',
			acmeEmail: 'certs@tudominio.cl',
			httpPort: 8080,
			httpsPort: 8443,
		})
	})

	it('cambiar los puertos recrea muelle-caddy: GET /proxy dice recreating, y después synced', async () => {
		await adminUseCases.updateProxyPorts({ http: '8080', https: '443' })
		await expect(ingressUseCases.getProxyStatus()).resolves.toMatchObject({
			status: 'recreating',
		})

		ingressDb.reset({ recreateMs: 0 })
		await adminUseCases.updateProxyPorts({ http: '8088', https: '443' })
		await expect(ingressUseCases.getProxyStatus()).resolves.toMatchObject({
			status: 'synced',
			caddy: { running: true, httpPort: 8088, httpsPort: 443 },
		})
	})

	it('con un puerto ocupado (8081 en el mock), muelle-caddy no arranca y GET /proxy dice por qué', async () => {
		ingressDb.reset({ recreateMs: 0 })
		await adminUseCases.updateProxyPorts({ http: '8081', https: '443' })

		await expect(ingressUseCases.getProxyStatus()).resolves.toMatchObject({
			status: 'out_of_sync',
			error: expect.stringContaining('8081'),
			caddy: { running: false },
		})
	})

	it('caddy_image no es un ajuste (§4.1): el mock lo rechaza en el PATCH y el panel no lo lee', async () => {
		const response = await fetch(api('/settings'), {
			method: 'PATCH',
			headers: { 'content-type': 'application/json' },
			body: JSON.stringify({ caddy_image: 'caddy:2' }),
		})
		expect(response.status).toBe(400)
		expect(await response.json()).toMatchObject({ error: { code: 'invalid_json' } })

		// Un backend que lo mandara igual no lo pone en los ajustes del panel.
		server.use(
			http.get(api('/settings'), () =>
				HttpResponse.json({
					require_2fa: false,
					cloudflare_token_set: false,
					cloudflare_zones: null,
					public_ip: '',
					public_ipv6: '',
					acme_email: '',
					panel_domain: 'panel.tudominio.cl',
					panel_allowed_cidrs: [],
					proxy_http_port: 80,
					proxy_https_port: 443,
					client_ip: '198.51.100.17',
					caddy_image: 'caddy:2',
				}),
			),
		)
		const settings = await adminUseCases.getSettings()
		expect(JSON.stringify(settings)).not.toContain('caddy')
	})

	it('"" borra la IP pública, la IPv6 y el email ACME', async () => {
		await adminUseCases.updatePublicIp({ ipv4: '', ipv6: '' })
		const settings = await adminUseCases.updateAcmeEmail('')

		expect(settings.proxy).toMatchObject({ publicIpv4: '', publicIpv6: '', acmeEmail: '' })
	})

	it('la IPv6 va en su forma canónica (el backend no la normaliza)', async () => {
		const settings = await adminUseCases.updatePublicIp({
			ipv4: '203.0.113.24',
			ipv6: '2001:DB8:0:0:0:0:0:24',
		})

		expect(settings.proxy?.publicIpv6).toBe('2001:db8::24')
		expect(adminDb.state.publicIpv6).toBe('2001:db8::24')
	})

	it('el token de Cloudflare se guarda con sus zonas, pero nunca vuelve', async () => {
		const token = 'cf_secreto_nuevo_de_cuarenta_caracteres'
		const settings = await adminUseCases.replaceCloudflareToken(token)

		expect(settings.proxy?.cloudflare).toEqual({ tokenSet: true, zones: 3 })
		expect(JSON.stringify(settings)).not.toContain(token)
		// En la auditoría queda que se guardó uno, no cuál.
		expect(JSON.stringify(adminDb.state.audit)).not.toContain(token)
		expect(adminDb.state.audit.at(-1)).toMatchObject({
			action: 'settings.update',
			details: { cloudflare_token: 'set' },
		})
	})

	it('quitar el token borra también las zonas', async () => {
		const settings = await adminUseCases.removeCloudflareToken()

		expect(settings.proxy?.cloudflare).toEqual({ tokenSet: false, zones: null })
		expect(adminDb.state.cloudflareToken).toBeNull()
		expect(adminDb.state.audit.at(-1)?.details).toEqual({ cloudflare_token: 'removed' })
	})

	it('un token que Cloudflare rechaza, o que no ve zonas, es un error del campo', async () => {
		for (const token of [
			'cf_token_rechazado_por_cloudflare_xx',
			'cf_token_sinzonas_visibles_en_la_cuenta',
		]) {
			const error = await adminUseCases
				.replaceCloudflareToken(token)
				.catch((caught: unknown) => caught)

			expect(error).toBeInstanceOf(ValidationError)
			expect((error as ValidationError).field).toBe('cloudflareToken')
		}
		expect(adminDb.state.cloudflareToken).toBe('cf_token_demo_de_cuarenta_caracteres_xx')
	})

	it('si Cloudflare no responde es CloudflareUnavailable (se reintenta), no un error del campo', async () => {
		const error = await adminUseCases
			.replaceCloudflareToken('cf_token_con_cloudflare_caido_xxxxxx')
			.catch((caught: unknown) => caught)

		expect(error).toBeInstanceOf(CloudflareUnavailable)
		expect(error).toBeInstanceOf(UnavailableError)
		expect(pendingPhase(error)).toBeNull()
	})
})

describe('auditoría', () => {
	it('pagina con before, de la más nueva a la más vieja, sin details', async () => {
		const first = await adminUseCases.listAudit()
		const pages = [first]
		let next = first.nextCursor
		while (next !== null) {
			const page = await adminUseCases.listAudit({ before: next })
			pages.push(page)
			next = page.nextCursor
		}

		expect(first.items).toHaveLength(AUDIT_PAGE_SIZE)
		expect(first.items[0]?.id).toBe(MOCK_AUDIT_SEED_COUNT)
		expect(first.nextCursor).toBe(String(MOCK_AUDIT_SEED_COUNT - AUDIT_PAGE_SIZE + 1))
		const ids = pages.flatMap((page) => page.items.map((event) => event.id))
		// Todos, una sola vez, en orden: el último pedido viene vacío o incompleto.
		expect(ids).toEqual(
			Array.from({ length: MOCK_AUDIT_SEED_COUNT }, (_, i) => MOCK_AUDIT_SEED_COUNT - i),
		)
		expect(first.items[0]).not.toHaveProperty('details')
	})

	it('lo primero es del primer arranque: install, con actor system (contrato §2.10 y §5.2)', async () => {
		const events = []
		let page = await adminUseCases.listAudit()
		events.push(...page.items)
		while (page.nextCursor !== null) {
			page = await adminUseCases.listAudit({ before: page.nextCursor })
			events.push(...page.items)
		}
		const oldest = events.slice(-2).reverse()
		expect(oldest.map((event) => [event.action, event.target, event.actor])).toEqual([
			['settings.init', 'settings', { kind: 'system' }],
			['registry.create', 'registry:ghcr.io', { kind: 'system' }],
		])
	})

	it('lo que se hace en el panel aparece arriba', async () => {
		await adminUseCases.updateAcmeEmail('certs@tudominio.cl')

		const page = await adminUseCases.listAudit()

		expect(page.items[0]).toMatchObject({
			action: 'settings.update',
			target: 'settings',
			actor: { kind: 'user', id: 1 },
		})
	})
})

describe('registries (Fase 2)', () => {
	it('agrega, cambia las credenciales y borra', async () => {
		await adminUseCases.addRegistry({
			host: 'registry.example.com:5000',
			username: 'bot',
			password: 'x',
		})
		await adminUseCases.updateRegistry({ host: 'ghcr.io', username: 'otro', password: 'y' })

		const list = await adminUseCases.listRegistries()
		expect(list.map(({ host, username }) => ({ host, username }))).toEqual([
			{ host: 'ghcr.io', username: 'otro' },
			{ host: 'registry.example.com:5000', username: 'bot' },
		])
		// Cambiar las credenciales mueve updated_at, no created_at.
		const [ghcr] = list
		expect(ghcr?.createdAt).toEqual(new Date('2026-10-01T12:00:00.000Z'))
		expect(ghcr?.updatedAt.getTime()).toBeGreaterThan(ghcr?.createdAt.getTime() ?? 0)

		await adminUseCases.removeRegistry('ghcr.io')
		await expect(adminUseCases.listRegistries()).resolves.toHaveLength(1)
	})

	it('vienen ordenados por host, con sus fechas', async () => {
		await adminUseCases.addRegistry({ host: 'docker.io', username: 'bot', password: 'x' })

		const list = await adminUseCases.listRegistries()

		expect(list.map((registry) => registry.host)).toEqual(['docker.io', 'ghcr.io'])
		expect(list[0]?.createdAt).toBeInstanceOf(Date)
	})

	it('el host se manda canónico: un alias de Docker Hub se guarda como docker.io', async () => {
		const registry = await adminUseCases.addRegistry({
			host: 'Index.Docker.IO',
			username: 'bot',
			password: 'x',
		})

		expect(registry.host).toBe('docker.io')
		expect(adminDb.state.registries.map((r) => r.host)).toContain('docker.io')
	})

	it('el backend rechaza un alias de Docker Hub en el campo host', async () => {
		const response = await fetch(api('/registries'), {
			method: 'POST',
			headers: { 'content-type': 'application/json' },
			body: JSON.stringify({ host: 'index.docker.io', username: 'a', password: 'b' }),
		})

		expect(response.status).toBe(422)
		expect(await response.json()).toMatchObject({
			error: { message: 'host: para Docker Hub usa docker.io' },
		})
	})

	it('el usuario se cuenta en caracteres, como el backend: 255 emoji pasan', async () => {
		const username = '😀'.repeat(255)

		await adminUseCases.updateRegistry({ host: 'ghcr.io', username, password: 'y' })

		expect(adminDb.state.registries.find((r) => r.host === 'ghcr.io')?.username).toBe(username)
	})

	it('el mock valida el host como validate.RegistryHost: hasta 255 y puerto de 1 a 65535', async () => {
		const post = (host: string) =>
			fetch(api('/registries'), {
				method: 'POST',
				headers: { 'content-type': 'application/json' },
				body: JSON.stringify({ host, username: 'a', password: 'b' }),
			})

		for (const host of [`${'a.'.repeat(127)}io`, 'r.io:0', 'r.io:70000']) {
			const response = await post(host)
			expect(response.status).toBe(422)
			expect(await response.json()).toMatchObject({
				error: { message: expect.stringMatching(/^host: /) },
			})
		}
		expect((await post('r.io:65535')).status).toBe(201)
	})

	it('cambiar las credenciales manda usuario y contraseña, los dos', async () => {
		let sent: unknown
		server.use(
			http.patch(api('/registries/:host'), async ({ request }) => {
				sent = await request.json()
				return HttpResponse.json({
					host: 'ghcr.io',
					username: 'otro',
					created_at: '2026-10-01T12:00:00.000Z',
					updated_at: '2026-10-14T12:00:00.000Z',
				})
			}),
		)

		await adminUseCases.updateRegistry({ host: 'ghcr.io', username: 'otro', password: 'y' })

		expect(sent).toEqual({ username: 'otro', password: 'y' })
	})

	it('el mock exige los dos campos en el PATCH, como el backend', async () => {
		const response = await fetch(api('/registries/ghcr.io'), {
			method: 'PATCH',
			headers: { 'content-type': 'application/json' },
			body: JSON.stringify({ username: 'otro' }),
		})

		expect(response.status).toBe(422)
	})

	it('cambiar las credenciales de un registry que ya no está es RegistryNotFound', async () => {
		await expect(
			adminUseCases.updateRegistry({ host: 'quay.io', username: 'a', password: 'b' }),
		).rejects.toBeInstanceOf(RegistryNotFound)
	})

	it('un host repetido es RegistryExists', async () => {
		await expect(
			adminUseCases.addRegistry({ host: 'ghcr.io', username: 'a', password: 'b' }),
		).rejects.toBeInstanceOf(RegistryExists)
	})

	it('contra el backend de la Fase 1, la ruta no existe: UnavailableError con fase 2', async () => {
		server.use(
			http.get(api('/registries'), () => apiError(404, 'no_route', 'no existe')),
			http.post(api('/registries'), () =>
				apiError(405, 'method_not_allowed', 'método no permitido'),
			),
		)

		const listError = await adminUseCases.listRegistries().catch((caught: unknown) => caught)
		const addError = await adminUseCases
			.addRegistry({ host: 'ghcr.io', username: 'a', password: 'b' })
			.catch((caught: unknown) => caught)

		expect(listError).toBeInstanceOf(UnavailableError)
		expect(pendingPhase(listError)).toBe(2)
		expect(pendingPhase(addError)).toBe(2)
	})

	it('un not_found no es la Fase 2: es el registry que ya no está', async () => {
		const error = await adminUseCases
			.removeRegistry('quay.io')
			.catch((caught: unknown) => caught)

		expect(error).toBeInstanceOf(RegistryNotFound)
		expect(pendingPhase(error)).toBeNull()
	})

	it('una ruta sin handler en los mocks contesta no_route, como el backend', async () => {
		server.use(http.get(api('/registries'), () => noRoute()))

		const error = await adminUseCases.listRegistries().catch((caught: unknown) => caught)

		expect(pendingPhase(error)).toBe(2)
	})

	it('un backend caído es UnavailableError sin fase (se reintenta)', async () => {
		server.use(http.get(api('/registries'), () => HttpResponse.error()))

		const error = await adminUseCases.listRegistries().catch((caught: unknown) => caught)

		expect(error).toBeInstanceOf(UnavailableError)
		expect(pendingPhase(error)).toBeNull()
	})
})
