import { http, HttpResponse } from 'msw'
import { afterAll, afterEach, beforeAll, beforeEach, describe, expect, it } from 'vitest'
import { api, apiError, invalid, noRoute } from '@/mocks/api'
import { adminDb, MOCK_AUDIT_SEED_COUNT } from '@/mocks/db/admin'
import { authDb, MOCK_USERNAME } from '@/mocks/db/auth'
import { identityDb } from '@/mocks/db/identity'
import { server } from '@/mocks/server'
import { pendingPhase, UnavailableError, ValidationError } from '@/shared/domain'
import { adminUseCases } from '../container'
import {
	AUDIT_PAGE_SIZE,
	InvalidCidr,
	RegistryExists,
	RegistryNotFound,
	TotpRequiredFirst,
} from '../domain'

// Recorre el módulo entero contra el backend simulado (src/mocks/handlers/admin.ts). Los
// mocks responden como el backend con las Fases 2 y 3; con `server.use` se simula el de la
// Fase 1, que todavía no las tiene.
beforeAll(() => server.listen({ onUnhandledRequest: 'error' }))
beforeEach(() => {
	authDb.reset()
	identityDb.reset()
	adminDb.reset()
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

	it('el token de Cloudflare se guarda pero nunca vuelve ni va a la auditoría', async () => {
		const settings = await adminUseCases.replaceCloudflareToken('cf-secreto-nuevo')

		expect(settings.proxy?.cloudflare.tokenSet).toBe(true)
		expect(JSON.stringify(settings)).not.toContain('cf-secreto-nuevo')
		expect(JSON.stringify(adminDb.state.audit)).not.toContain('cf-secreto-nuevo')
		expect(adminDb.state.audit.at(-1)).toMatchObject({ action: 'settings.update' })
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

		await expect(adminUseCases.listRegistries()).resolves.toEqual([
			{ host: 'ghcr.io', username: 'otro' },
			{ host: 'registry.example.com:5000', username: 'bot' },
		])

		await adminUseCases.removeRegistry('ghcr.io')
		await expect(adminUseCases.listRegistries()).resolves.toHaveLength(1)
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
