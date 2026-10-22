import type { QueryClient } from '@tanstack/svelte-query'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { render } from 'vitest-browser-svelte'
import { createQueryClient } from '@/shared/data/query'
import { formatDateTime } from '@/shared/ui'
import { authKeys } from '@/modules/auth'
import { CloudflareUnavailable, type Settings } from '../../../../domain'
import SettingsView from './SettingsView.svelte'

const mocks = vi.hoisted(() => ({
	getSettings: vi.fn(),
	listRegistries: vi.fn(),
	listAudit: vi.fn(),
	listUsers: vi.fn(),
	listTokens: vi.fn(),
	setCloudflareOriginCa: vi.fn(),
}))

vi.mock('@/modules/admin/container', () => ({
	adminUseCases: {
		getSettings: mocks.getSettings,
		updateSecurity: vi.fn(),
		updatePublicIp: vi.fn(),
		updateAcmeEmail: vi.fn(),
		updateProxyPorts: vi.fn(),
		replaceCloudflareToken: vi.fn(),
		removeCloudflareToken: vi.fn(),
		setCloudflareOriginCa: mocks.setCloudflareOriginCa,
		listAudit: mocks.listAudit,
		listRegistries: mocks.listRegistries,
		addRegistry: vi.fn(),
		updateRegistry: vi.fn(),
		removeRegistry: vi.fn(),
	},
}))

// "Proxy y DNS" pide `GET /proxy` (ingress) al abrirse, por si muelle volvió a los puertos
// anteriores.
const ingress = vi.hoisted(() => ({ getProxyStatus: vi.fn() }))
vi.mock('@/modules/ingress/container', () => ({ ingressUseCases: ingress }))

// El origen de la página: el de vitest, salvo que un test entre por una dirección de Caddy
// (`https://<panel_domain>[:puerto]`), que no es la de vitest.
const page = vi.hoisted(() => ({ origin: null as string | null }))
vi.mock('@/modules/admin/ui/features/proxySettings/currentOrigin', () => ({
	currentOrigin: () => page.origin ?? location.origin,
}))

// Usuarios es de identity.
vi.mock('@/modules/identity/container', () => ({
	identityUseCases: {
		listUsers: mocks.listUsers,
		deleteUser: vi.fn(),
		resetUserTotp: vi.fn(),
		createInvite: vi.fn(),
		listTokens: mocks.listTokens,
		createToken: vi.fn(),
		revokeToken: vi.fn(),
		listScopeProjects: vi.fn(),
		listScopeStages: vi.fn(),
	},
}))

const SETTINGS: Settings = {
	require2fa: false,
	proxy: {
		cloudflare: { tokenSet: true, zones: 4, originCa: false },
		publicIpv4: '203.0.113.24',
		publicIpv6: '',
		acmeEmail: 'ops@tudominio.cl',
		panelDomain: 'panel.tudominio.cl',
		httpPort: 80,
		httpsPort: 443,
	},
	panelAccess: { allowedCidrs: [], clientIp: '198.51.100.17' },
}

function client(): QueryClient {
	const queryClient = createQueryClient({ retry: false })
	queryClient.setQueryData(authKeys.me(), {
		username: 'felipe',
		totpEnabled: true,
		needsTotpSetup: false,
	})
	return queryClient
}

describe('SettingsScreen', () => {
	beforeEach(() => {
		vi.clearAllMocks()
		localStorage.clear()
		page.origin = null
		ingress.getProxyStatus.mockResolvedValue({
			status: 'synced',
			error: null,
			syncedAt: new Date('2026-10-16T12:00:00Z'),
			caddy: { image: null, running: true, httpPort: 80, httpsPort: 443 },
			portsRollback: null,
			panelCertificate: null,
		})
		mocks.listAudit.mockResolvedValue({ items: [], nextCursor: null })
		mocks.listUsers.mockResolvedValue([
			{ username: 'felipe', totpEnabled: true, createdAt: new Date('2026-10-09T12:00:00Z') },
		])
		mocks.listTokens.mockResolvedValue([])
	})

	it('Proxy y DNS muestra sus tarjetas, y nada de las otras vistas', async () => {
		mocks.getSettings.mockResolvedValue(SETTINGS)

		const screen = await render(SettingsView, { client: client(), section: 'proxy' })

		await expect.element(screen.getByRole('heading', { name: 'Cloudflare' })).toBeVisible()
		await expect.element(screen.getByText('4 zonas visibles · cifrado')).toBeVisible()
		await expect.element(screen.getByRole('button', { name: 'Quitar token' })).toBeVisible()
		await expect
			.element(screen.getByRole('heading', { name: 'Puertos del proxy' }))
			.toBeVisible()
		expect(screen.container.textContent).not.toContain('IPs permitidas para el panel')
		expect(mocks.listRegistries).not.toHaveBeenCalled()
		expect(mocks.listUsers).not.toHaveBeenCalled()
		expect(mocks.listAudit).not.toHaveBeenCalled()
	})

	it('Seguridad muestra solo su tarjeta', async () => {
		mocks.getSettings.mockResolvedValue(SETTINGS)

		const screen = await render(SettingsView, { client: client(), section: 'security' })

		await expect.element(screen.getByLabelText('IPs permitidas para el panel')).toBeVisible()
		expect(screen.container.textContent).not.toContain('Cloudflare')
	})

	it('Registries, Usuarios y Auditoría se ven sin pedir los settings', async () => {
		const at = new Date('2026-10-09T15:30:00.000Z')
		mocks.listRegistries.mockResolvedValue([
			{ host: 'ghcr.io', username: 'cpu-commits', createdAt: at, updatedAt: at },
		])

		const registries = await render(SettingsView, { client: client(), section: 'registries' })
		await expect.element(registries.getByText('ghcr.io')).toBeVisible()
		registries.unmount()

		const users = await render(SettingsView, { client: client(), section: 'users' })
		await expect.element(users.getByRole('heading', { name: 'Usuarios' })).toBeVisible()
		users.unmount()

		const audit = await render(SettingsView, { client: client(), section: 'audit' })
		await expect.element(audit.getByRole('heading', { name: 'Auditoría' })).toBeVisible()

		expect(mocks.getSettings).not.toHaveBeenCalled()
	})

	it('el aviso de la dirección nueva del panel no pide tocar la configuración de muelle', async () => {
		// El origen permitido sigue al puerto HTTPS de Ajustes (contrato §0.1 y §4.1): no hay
		// que actualizar MUELLE_PANEL_ORIGIN en el servidor.
		localStorage.setItem('muelle:admin:panel-moved', 'https://panel.tudominio.cl:8443')
		mocks.getSettings.mockResolvedValue({
			...SETTINGS,
			proxy: { ...SETTINGS.proxy, httpsPort: 8443 },
		})

		const screen = await render(SettingsView, { client: client() })

		await expect
			.element(
				screen.getByText('La dirección del panel ahora es https://panel.tudominio.cl:8443'),
			)
			.toBeVisible()
		expect(document.body.textContent).not.toContain('MUELLE_PANEL_ORIGIN')
		expect(document.body.textContent).not.toContain('docker service update')
		// El panel no puede saber si la dirección nueva responde (la CSP no deja pedir a otro
		// origen): el aviso dice qué hacer si no (contrato §3.5).
		await expect
			.element(screen.getByText(/Si la nueva no responde en un minuto, vuelve a la anterior/))
			.toBeVisible()
	})

	it('si muelle volvió a los puertos anteriores, lo dice en Puertos, sin la dirección de Caddy si no se entró por ella', async () => {
		mocks.getSettings.mockResolvedValue(SETTINGS)
		ingress.getProxyStatus.mockResolvedValue({
			status: 'synced',
			error: null,
			syncedAt: new Date('2026-10-16T12:00:00Z'),
			caddy: { image: null, running: true, httpPort: 80, httpsPort: 443 },
			portsRollback: {
				attempted: { httpPort: 80, httpsPort: 9443 },
				restored: { httpPort: 80, httpsPort: 443 },
				error: 'los puertos 80 o 9443 del host ya están en uso, y muelle-caddy no pudo arrancar con ellos',
				at: new Date(Date.now() - 2 * 60 * 60_000),
			},
		})

		const screen = await render(SettingsView, { client: client() })

		// Dice cuándo: GET /proxy lo trae hasta el próximo cambio de puertos.
		const ports = screen.getByRole('region', { name: 'Puertos del proxy' })
		await expect
			.element(
				ports.getByText(
					'muelle volvió a los puertos anteriores (HTTP 80, HTTPS 443) hace 2 horas.',
				),
			)
			.toBeVisible()
		await expect
			.element(ports.getByText(/HTTPS 9443: los puertos 80 o 9443 del host ya están en uso/))
			.toBeVisible()
		// Este panel está en otra dirección (vitest), no en la de Caddy: no la nombra ni la
		// ofrece, nunca la usó.
		expect(document.body.textContent).not.toContain('https://panel.tudominio.cl')
		await expect
			.element(ports.getByRole('link', { name: 'Volver a la dirección anterior' }))
			.not.toBeInTheDocument()
	})

	it('en la dirección que falló, Puertos ofrece volver a la anterior: Proxy y DNS allá', async () => {
		// Una pestaña que quedó en la dirección nueva: muelle-caddy no arrancó con el 8443 y
		// muelle volvió al 443 (contrato §3.5).
		page.origin = 'https://panel.tudominio.cl:8443'
		mocks.getSettings.mockResolvedValue(SETTINGS)
		ingress.getProxyStatus.mockResolvedValue({
			status: 'synced',
			error: null,
			syncedAt: new Date('2026-10-16T12:00:00Z'),
			caddy: { image: null, running: true, httpPort: 80, httpsPort: 443 },
			portsRollback: {
				attempted: { httpPort: 80, httpsPort: 8443 },
				restored: { httpPort: 80, httpsPort: 443 },
				error: 'el puerto 8443 del host ya está en uso, y muelle-caddy no pudo arrancar con él',
				at: new Date(Date.now() - 60_000),
			},
			panelCertificate: null,
		})

		const screen = await render(SettingsView, { client: client() })

		const ports = screen.getByRole('region', { name: 'Puertos del proxy' })
		await expect
			.element(
				ports.getByText(
					/La dirección del panel sigue siendo https:\/\/panel\.tudominio\.cl\./,
				),
			)
			.toBeVisible()
		const back = ports.getByRole('link', { name: 'Volver a la dirección anterior' })
		await expect.element(back).toBeVisible()
		await expect
			.element(back)
			.toHaveAttribute('href', 'https://panel.tudominio.cl/settings/proxy')
	})

	it('el switch de los certificados de Cloudflare muestra lo guardado: apagarlo pide confirmar, y cancelar lo deja encendido', async () => {
		mocks.getSettings.mockResolvedValue({
			...SETTINGS,
			proxy: {
				...SETTINGS.proxy,
				cloudflare: { tokenSet: true, zones: 4, originCa: true },
			},
		})

		const screen = await render(SettingsView, { client: client() })

		const toggle = screen.getByRole('switch', {
			name: /Certificados de Cloudflare \(Origin CA\)/,
		})
		await expect.element(toggle).toBeChecked()
		await toggle.click()

		const dialog = screen.getByRole('dialog', { name: "¿Volver a Let's Encrypt?" })
		await expect.element(dialog).toBeVisible()
		await expect.element(toggle).toBeChecked()
		await dialog.getByRole('button', { name: 'Cancelar' }).click()
		await expect.element(dialog).not.toBeInTheDocument()
		await expect.element(toggle).toBeChecked()
		expect(mocks.setCloudflareOriginCa).not.toHaveBeenCalled()
	})

	it('con los certificados de Cloudflare encendidos, la tarjeta dice cuál usa el panel y el aviso de su falla', async () => {
		mocks.getSettings.mockResolvedValue({
			...SETTINGS,
			proxy: {
				...SETTINGS.proxy,
				cloudflare: { tokenSet: true, zones: 4, originCa: true },
			},
		})
		const error =
			"muelle no pudo emitir el certificado de Cloudflare: el token no tiene el permiso Zone → SSL and Certificates → Edit en la zona de panel.tudominio.cl (Cloudflare: Authentication error); agrégalo al token: muelle lo reintenta solo cada 6 h; mientras tanto usa Let's Encrypt"
		ingress.getProxyStatus.mockResolvedValue({
			status: 'synced',
			error: null,
			syncedAt: new Date('2026-10-16T12:00:00Z'),
			caddy: { image: null, running: true, httpPort: 80, httpsPort: 443 },
			portsRollback: null,
			panelCertificate: {
				hostname: 'panel.tudominio.cl',
				certSource: 'letsencrypt',
				originCa: {
					status: 'failed',
					errorCode: 'permission',
					error,
					retryAt: new Date(),
					expiresAt: null,
				},
			},
		})

		const screen = await render(SettingsView, { client: client() })

		const cloudflare = screen.getByRole('region', { name: 'Cloudflare' })
		await expect.element(cloudflare.getByText("Panel: Let's Encrypt")).toBeVisible()
		await expect
			.element(
				cloudflare.getByText('No se pudo emitir el certificado de Cloudflare del panel'),
			)
			.toBeVisible()
		await expect.element(cloudflare.getByText(`M${error.slice(1)}`)).toBeVisible()
		// El panel no tiene Reintentar: se reintenta solo.
		await expect
			.element(cloudflare.getByRole('button', { name: 'Reintentar' }))
			.not.toBeInTheDocument()
	})

	it('si encenderlos falla, el switch sigue apagado y la tarjeta dice por qué', async () => {
		mocks.getSettings.mockResolvedValue(SETTINGS)
		mocks.setCloudflareOriginCa.mockRejectedValueOnce(new CloudflareUnavailable())

		const screen = await render(SettingsView, { client: client() })

		const toggle = screen.getByRole('switch', {
			name: /Certificados de Cloudflare \(Origin CA\)/,
		})
		await expect.element(toggle).not.toBeChecked()
		await toggle.click()

		const cloudflare = screen.getByRole('region', { name: 'Cloudflare' })
		await expect
			.element(
				cloudflare.getByText(
					'Cloudflare no respondió: no se cambió nada. Vuelve a intentarlo en un momento.',
				),
			)
			.toBeVisible()
		expect(mocks.setCloudflareOriginCa).toHaveBeenCalledOnce()
		await expect.element(toggle).not.toBeChecked()
	})

	it('cuándo se actualizó un registry es un <time> con la fecha, y la fecha exacta se lee', async () => {
		mocks.getSettings.mockResolvedValue(SETTINGS)
		const at = new Date('2026-10-09T15:30:00.000Z')
		mocks.listRegistries.mockResolvedValue([
			{ host: 'ghcr.io', username: 'cpu-commits', createdAt: at, updatedAt: at },
		])

		const screen = await render(SettingsView, { client: client(), section: 'registries' })

		const updated = screen.getByText(/^Actualizado /)
		await expect.element(updated).toBeVisible()
		const time = updated.element().closest('time')
		expect(time?.getAttribute('datetime')).toBe('2026-10-09T15:30:00.000Z')
		// La fecha exacta no queda solo en un `title` (ni un lector de pantalla ni un dedo la
		// ven): va en el texto de la fila, para el lector.
		const row = time?.parentElement
		expect(row?.textContent).toContain(formatDateTime(at))
	})
})
