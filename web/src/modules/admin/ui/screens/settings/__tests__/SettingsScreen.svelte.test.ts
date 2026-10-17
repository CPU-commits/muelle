import type { QueryClient } from '@tanstack/svelte-query'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { render } from 'vitest-browser-svelte'
import { createQueryClient } from '@/shared/data/query'
import { formatDateTime } from '@/shared/ui'
import { authKeys } from '@/modules/auth'
import { UnavailableError } from '@/shared/domain'
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

// Usuarios es de identity y la auditoría lee los nombres de sus tokens.
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

const PHASE_1: Settings = { require2fa: false, proxy: null, panelAccess: null }

const PHASE_3: Settings = {
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

	it('contra el backend de la Fase 1, las secciones de las Fases 2 y 3 dicen cuándo llegan', async () => {
		mocks.getSettings.mockResolvedValue(PHASE_1)
		mocks.listRegistries.mockRejectedValue(new UnavailableError('no_route', { phase: 2 }))

		const screen = await render(SettingsView, { client: client() })

		await expect.element(screen.getByText('Proxy y DNS llega con la Fase 3')).toBeVisible()
		await expect
			.element(screen.getByText('La lista de IPs permitidas llega con la Fase 3'))
			.toBeVisible()
		await expect
			.element(screen.getByText('La sección de registries llega con la Fase 2'))
			.toBeVisible()
		// Lo de la Fase 1 sí está, y los registries no ofrecen "Agregar".
		await expect.element(screen.getByRole('switch', { name: /Exigir 2FA/ })).toBeVisible()
		await expect.element(screen.getByRole('heading', { name: 'Usuarios' })).toBeVisible()
		await expect
			.element(screen.getByRole('button', { name: 'Agregar' }))
			.not.toBeInTheDocument()
		await expect
			.element(screen.getByRole('heading', { name: 'Cloudflare' }))
			.not.toBeInTheDocument()
	})

	it('con las Fases 2 y 3, las secciones se muestran en vez del aviso', async () => {
		mocks.getSettings.mockResolvedValue(PHASE_3)
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
		await expect.element(screen.getByText(/llega con la Fase/)).not.toBeInTheDocument()
	})

	it('el aviso de la dirección nueva del panel no pide tocar la configuración de muelle', async () => {
		// El origen permitido sigue al puerto HTTPS de Ajustes (contrato §0.1 y §4.1): no hay
		// que actualizar MUELLE_PANEL_ORIGIN en el servidor.
		localStorage.setItem('muelle:admin:panel-moved', 'https://panel.tudominio.cl:8443')
		mocks.getSettings.mockResolvedValue({
			...PHASE_3,
			proxy: PHASE_3.proxy && { ...PHASE_3.proxy, httpsPort: 8443 },
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
	})

	it('si muelle volvió a los puertos anteriores, lo dice en Puertos y ofrece la dirección anterior', async () => {
		mocks.getSettings.mockResolvedValue(PHASE_3)
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
				at: new Date('2026-10-16T20:41:07.120Z'),
			},
		})

		const screen = await render(SettingsView, { client: client() })

		const ports = screen.getByRole('region', { name: 'Puertos del proxy' })
		await expect
			.element(
				ports.getByText('muelle volvió a los puertos anteriores (HTTP 80, HTTPS 443).'),
			)
			.toBeVisible()
		await expect
			.element(
				ports.getByText(
					/La dirección del panel sigue siendo https:\/\/panel\.tudominio\.cl\./,
				),
			)
			.toBeVisible()
		await expect
			.element(ports.getByRole('link', { name: 'Volver a la dirección anterior' }))
			.toHaveAttribute('href', 'https://panel.tudominio.cl/ajustes')
	})

	it('cuándo se actualizó un registry es un <time> con la fecha, y la fecha exacta se lee', async () => {
		mocks.getSettings.mockResolvedValue(PHASE_3)
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
