import type { QueryClient } from '@tanstack/svelte-query'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { render } from 'vitest-browser-svelte'
import { createQueryClient } from '@/shared/data/query'
import { formatDateTime } from '@/shared/ui'
import { authKeys } from '@/modules/auth'
import type { Settings } from '../../../../domain'
import SettingsView from './SettingsView.svelte'

const mocks = vi.hoisted(() => ({
	getSettings: vi.fn(),
	listRegistries: vi.fn(),
	listAudit: vi.fn(),
	listUsers: vi.fn(),
	listTokens: vi.fn(),
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
		cloudflare: { tokenSet: true, zones: 4 },
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
		ingress.getProxyStatus.mockResolvedValue({
			status: 'synced',
			error: null,
			syncedAt: new Date('2026-10-16T12:00:00Z'),
			caddy: { image: null, running: true, httpPort: 80, httpsPort: 443 },
			portsRollback: null,
		})
		mocks.listAudit.mockResolvedValue({ items: [], nextCursor: null })
		mocks.listUsers.mockResolvedValue([
			{ username: 'felipe', totpEnabled: true, createdAt: new Date('2026-10-09T12:00:00Z') },
		])
		mocks.listTokens.mockResolvedValue([])
	})

	it('muestra todas las secciones', async () => {
		mocks.getSettings.mockResolvedValue(SETTINGS)
		const at = new Date('2026-10-09T15:30:00.000Z')
		mocks.listRegistries.mockResolvedValue([
			{ host: 'ghcr.io', username: 'cpu-commits', createdAt: at, updatedAt: at },
		])

		const screen = await render(SettingsView, { client: client() })

		await expect.element(screen.getByRole('heading', { name: 'Cloudflare' })).toBeVisible()
		await expect.element(screen.getByText('4 zonas visibles · cifrado')).toBeVisible()
		await expect.element(screen.getByRole('button', { name: 'Quitar token' })).toBeVisible()
		await expect.element(screen.getByText(/^Actualizado /)).toBeVisible()
		await expect.element(screen.getByLabelText('IPs permitidas para el panel')).toBeVisible()
		await expect.element(screen.getByText('ghcr.io')).toBeVisible()
	})

	it('el aviso de la dirección nueva del panel no pide tocar la configuración de muelle', async () => {
		// El origen permitido sigue al puerto HTTPS de Ajustes (contrato §0.1 y §4.1): no hay
		// que actualizar MUELLE_PANEL_ORIGIN en el servidor.
		localStorage.setItem('muelle:admin:panel-moved', 'https://panel.tudominio.cl:8443')
		mocks.getSettings.mockResolvedValue({
			...SETTINGS,
			proxy: { ...SETTINGS.proxy, httpsPort: 8443 },
		})
		mocks.listRegistries.mockResolvedValue([])

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
		mocks.listRegistries.mockResolvedValue([])
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

	it('cuándo se actualizó un registry es un <time> con la fecha, y la fecha exacta se lee', async () => {
		mocks.getSettings.mockResolvedValue(SETTINGS)
		const at = new Date('2026-10-09T15:30:00.000Z')
		mocks.listRegistries.mockResolvedValue([
			{ host: 'ghcr.io', username: 'cpu-commits', createdAt: at, updatedAt: at },
		])

		const screen = await render(SettingsView, { client: client() })

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
