import { http, HttpResponse } from 'msw'
import { afterAll, afterEach, beforeAll, beforeEach, describe, expect, it } from 'vitest'
import { api, apiError, invalid, noRoute } from '@/mocks/api'
import { adminDb, MOCK_AUDIT_SEED_COUNT } from '@/mocks/db/admin'
import { authDb, MOCK_USERNAME } from '@/mocks/db/auth'
import { identityDb } from '@/mocks/db/identity'
import { ingressDb } from '@/mocks/db/ingress'
import { server } from '@/mocks/server'
import { UnavailableError, UnexpectedError, ValidationError } from '@/shared/domain'
import { ingressUseCases } from '@/modules/ingress'
import { adminUseCases } from '../container'
import {
	AUDIT_PAGE_SIZE,
	CloudflareUnavailable,
	InvalidCidr,
	PortsDockerUnavailable,
	RegistryExists,
	RegistryNotFound,
	TotpRequiredFirst,
	WouldLockOut,
} from '../domain'

// Recorre el módulo entero contra el backend simulado (src/mocks/handlers/admin.ts), que
// responde como el backend.
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

/** Un `GET /settings` completo (contrato §4: todas las claves, siempre). */
const SETTINGS_DTO = {
	require_2fa: true,
	cloudflare_token_set: false,
	cloudflare_zones: null,
	cloudflare_origin_ca: false,
	public_ip: '',
	public_ipv6: '',
	acme_email: '',
	panel_domain: 'panel.tudominio.cl',
	proxy_http_port: 80,
	proxy_https_port: 443,
	panel_allowed_cidrs: [],
	client_ip: '198.51.100.17',
}

describe('settings', () => {
	it('trae el 2FA, proxy, DNS, certificados e IPs permitidas', async () => {
		await expect(adminUseCases.getSettings()).resolves.toEqual({
			require2fa: false,
			proxy: {
				cloudflare: { tokenSet: true, zones: 4, originCa: false },
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

	it('un GET /settings sin las claves del proxy no es del contrato (ContractError)', async () => {
		server.use(http.get(api('/settings'), () => HttpResponse.json({ require_2fa: false })))

		await expect(adminUseCases.getSettings()).rejects.toMatchObject({ name: 'ContractError' })
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

	it('exigir 2FA solo manda require_2fa: lo demás no se toca', async () => {
		enableOwnTotp()
		let sent: unknown
		server.use(
			http.patch(api('/settings'), async ({ request }) => {
				sent = await request.json()
				return HttpResponse.json(SETTINGS_DTO)
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

	it('el mismo puerto para HTTP y HTTPS es 422 en proxy_https_port, con el texto del backend', async () => {
		const response = await fetch(api('/settings'), {
			method: 'PATCH',
			headers: { 'content-type': 'application/json' },
			body: JSON.stringify({ proxy_http_port: 8443, proxy_https_port: 8443 }),
		})
		expect(response.status).toBe(422)
		// src/internal/admin/app/usecases/settings.go
		expect(await response.json()).toMatchObject({
			error: {
				code: 'invalid',
				message: 'proxy_https_port: no puede ser el mismo que proxy_http_port (8443)',
			},
		})
	})

	it('el backend rechaza { y } en el email ACME y en el token (422 en ese campo) y no guarda nada', async () => {
		// Directo contra el endpoint, sin el chequeo del panel: el 422 lo tiene que dar el mock.
		const patch = (body: Record<string, unknown>) =>
			fetch(api('/settings'), {
				method: 'PATCH',
				headers: { 'content-type': 'application/json' },
				body: JSON.stringify(body),
			})
		const before = {
			email: adminDb.state.acmeEmail,
			token: adminDb.state.cloudflareToken,
			audit: adminDb.state.audit.length,
		}

		const email = await patch({ acme_email: 'a{x}@tudominio.cl' })
		expect(email.status).toBe(422)
		expect(await email.json()).toMatchObject({
			error: {
				code: 'invalid',
				message: 'acme_email: no puede tener { ni }: Caddy los toma como placeholders',
			},
		})

		// Con la forma de un token, el de los placeholders; sin ella, el de la forma (va antes).
		const token = 'cf_token_con_un_{placeholder}_adentro'
		const shaped = await patch({ cloudflare_token: token })
		expect(shaped.status).toBe(422)
		const body = await shaped.text()
		expect(JSON.parse(body)).toMatchObject({
			error: {
				message:
					'cloudflare_token: no puede tener { ni }: Caddy los toma como placeholders',
			},
		})
		expect(body).not.toContain(token)
		const short = await patch({ cloudflare_token: 'a{b}' })
		// El mismo texto que el backend (src/internal/admin/app/usecases/settings.go).
		expect(await short.json()).toMatchObject({
			error: {
				message:
					'cloudflare_token: no parece un token de API de Cloudflare (de 20 a 512 caracteres visibles, sin espacios)',
			},
		})

		expect(adminDb.state.acmeEmail).toBe(before.email)
		expect(adminDb.state.cloudflareToken).toBe(before.token)
		expect(adminDb.state.audit).toHaveLength(before.audit)
	})

	it('el 422 de { y } que vuelve del backend llega al campo con su motivo', async () => {
		const reason = 'no puede tener { ni }: Caddy los toma como placeholders'
		server.use(http.patch(api('/settings'), () => invalid('acme_email', reason)))
		const email = await adminUseCases
			.updateAcmeEmail('certs@tudominio.cl')
			.catch((caught: unknown) => caught)
		expect(email).toBeInstanceOf(ValidationError)
		expect(email).toMatchObject({ field: 'acmeEmail', message: reason })

		server.use(http.patch(api('/settings'), () => invalid('cloudflare_token', reason)))
		const token = await adminUseCases
			.replaceCloudflareToken('cf_token_de_mas_de_veinte_caracteres')
			.catch((caught: unknown) => caught)
		expect(token).toBeInstanceOf(ValidationError)
		expect(token).toMatchObject({ field: 'cloudflareToken', message: reason })
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
			// Sin contenedor, pero los puertos son siempre los de los ajustes (contrato §3.3).
			caddy: { image: null, running: false, httpPort: 8080, httpsPort: 443 },
		})

		ingressDb.reset({ recreateMs: 0 })
		await adminUseCases.updateProxyPorts({ http: '8088', https: '443' })
		await expect(ingressUseCases.getProxyStatus()).resolves.toMatchObject({
			status: 'synced',
			caddy: { running: true, httpPort: 8088, httpsPort: 443 },
		})
	})

	it('un puerto ocupado en el host (8081 en el mock) es 422 en su campo, con el motivo, y no se guarda nada', async () => {
		// CheckProxyPorts (contrato §3.5) prueba los puertos nuevos antes de guardar.
		const http = await adminUseCases
			.updateProxyPorts({ http: '8081', https: '8443' })
			.catch((caught: unknown) => caught)
		expect(http).toBeInstanceOf(ValidationError)
		expect(http).toMatchObject({
			field: 'httpPort',
			message: 'el puerto 8081 del host ya está en uso: libéralo o elige otro',
		})

		// El HTTPS se prueba en TCP y en UDP (HTTP/3).
		const https = await adminUseCases
			.updateProxyPorts({ http: '80', https: '8081' })
			.catch((caught: unknown) => caught)
		expect(https).toMatchObject({
			field: 'httpsPort',
			message:
				'el puerto 8081 del host ya está en uso (en TCP o en UDP): libéralo o elige otro',
		})

		const settings = await adminUseCases.getSettings()
		expect(settings.proxy).toMatchObject({ httpPort: 80, httpsPort: 443 })
		await expect(ingressUseCases.getProxyStatus()).resolves.toMatchObject({ status: 'synced' })
	})

	it('sin el proxy de muelle (sin dominio del panel), CheckProxyPorts no prueba nada: se guarda', async () => {
		// Contrato §3.5: con el proxy deshabilitado no toca Docker y devuelve nil.
		adminDb.reset({ panelDomain: '' })
		const settings = await adminUseCases.updateProxyPorts({ http: '8081', https: '8443' })
		expect(settings.proxy).toMatchObject({ httpPort: 8081, httpsPort: 8443 })
	})

	it('si el puerto se ocupa después de la prueba (9443 en el mock), muelle vuelve a los anteriores (contrato §3.5)', async () => {
		ingressDb.reset({ recreateMs: 0 })
		// El PATCH lo prueba y está libre: se guarda.
		const saved = await adminUseCases.updateProxyPorts({ http: '80', https: '9443' })
		expect(saved.proxy).toMatchObject({ httpPort: 80, httpsPort: 9443 })

		// Al recrear, otro ya lo tomó: muelle-caddy no arranca y muelle vuelve a 80/443.
		const proxy = await ingressUseCases.getProxyStatus()
		expect(proxy).toMatchObject({
			status: 'synced',
			caddy: { running: true, httpPort: 80, httpsPort: 443 },
			portsRollback: {
				attempted: { httpPort: 80, httpsPort: 9443 },
				restored: { httpPort: 80, httpsPort: 443 },
				error: expect.stringContaining('9443'),
			},
		})
		expect(proxy.portsRollback?.at).toBeInstanceOf(Date)
		const settings = await adminUseCases.getSettings()
		expect(settings.proxy).toMatchObject({ httpPort: 80, httpsPort: 443 })
		// Queda en la auditoría, del actor system.
		const [last] = (await adminUseCases.listAudit()).items
		expect(last).toMatchObject({ action: 'settings.ports_rollback', actor: { kind: 'system' } })

		// El próximo cambio de puertos es un intento nuevo: limpia ports_rollback.
		await adminUseCases.updateProxyPorts({ http: '8080', https: '443' })
		await expect(ingressUseCases.getProxyStatus()).resolves.toMatchObject({
			portsRollback: null,
			caddy: { httpPort: 8080, httpsPort: 443 },
		})
	})

	it('los ajustes ya dicen los puertos anteriores aunque nadie haya preguntado GET /proxy', async () => {
		ingressDb.reset({ recreateMs: 0 })
		await adminUseCases.updateProxyPorts({ http: '9443', https: '443' })
		const settings = await adminUseCases.getSettings()
		expect(settings.proxy).toMatchObject({ httpPort: 80, httpsPort: 443 })
	})

	it('Docker caído al probar los puertos nuevos (503 docker_unavailable) es PortsDockerUnavailable', async () => {
		server.use(
			http.patch(api('/settings'), () =>
				apiError(503, 'docker_unavailable', 'Docker no responde'),
			),
		)
		await expect(
			adminUseCases.updateProxyPorts({ http: '8080', https: '443' }),
		).rejects.toBeInstanceOf(PortsDockerUnavailable)
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
					cloudflare_origin_ca: false,
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

		expect(settings.proxy?.cloudflare).toEqual({ tokenSet: true, zones: 3, originCa: false })
		expect(JSON.stringify(settings)).not.toContain(token)
		// En la auditoría queda que se guardó uno, no cuál.
		expect(JSON.stringify(adminDb.state.audit)).not.toContain(token)
		expect(adminDb.state.audit.at(-1)).toMatchObject({
			action: 'settings.update',
			details: { cloudflare_token: 'set' },
		})
	})

	it('quitar el token borra también las zonas', async () => {
		const settings = await adminUseCases.removeCloudflareToken({
			tokenSet: true,
			zones: 3,
			originCa: false,
		})

		expect(settings.proxy?.cloudflare).toEqual({
			tokenSet: false,
			zones: null,
			originCa: false,
		})
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
			actor: { kind: 'user', id: 1, name: 'felipe' },
		})
	})

	it('cada evento trae el nombre de quién lo hizo, copiado al escribirlo (contrato §4.2)', async () => {
		const events = []
		let page = await adminUseCases.listAudit()
		events.push(...page.items)
		while (page.nextCursor) {
			page = await adminUseCases.listAudit({ before: page.nextCursor })
			events.push(...page.items)
		}
		const actors = new Set(events.map((event) => JSON.stringify(event.actor)))
		expect(actors).toContain(JSON.stringify({ kind: 'user', id: 1, name: 'felipe' }))
		expect(actors).toContain(JSON.stringify({ kind: 'token', id: 1, name: 'github-actions' }))
		expect(actors).toContain(JSON.stringify({ kind: 'system' }))
		// Uno de antes de la migración 0004, de un token que ya no existía: sin nombre.
		expect(actors).toContain(JSON.stringify({ kind: 'token', id: 9, name: '' }))
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

	it('una ruta que no existe (no_route, method_not_allowed) es un error inesperado, no un registry que falta', async () => {
		server.use(
			http.get(api('/registries'), () => noRoute()),
			http.post(api('/registries'), () =>
				apiError(405, 'method_not_allowed', 'método no permitido'),
			),
		)

		const listError = await adminUseCases.listRegistries().catch((caught: unknown) => caught)
		const addError = await adminUseCases
			.addRegistry({ host: 'ghcr.io', username: 'a', password: 'b' })
			.catch((caught: unknown) => caught)

		expect(listError).toBeInstanceOf(UnexpectedError)
		expect(addError).toBeInstanceOf(UnexpectedError)
	})

	it('un not_found es el registry que ya no está', async () => {
		const error = await adminUseCases
			.removeRegistry('quay.io')
			.catch((caught: unknown) => caught)

		expect(error).toBeInstanceOf(RegistryNotFound)
	})

	it('un backend caído es UnavailableError (se reintenta)', async () => {
		server.use(http.get(api('/registries'), () => HttpResponse.error()))

		const error = await adminUseCases.listRegistries().catch((caught: unknown) => caught)

		expect(error).toBeInstanceOf(UnavailableError)
	})
})
