import type { QueryClient } from '@tanstack/svelte-query'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { render } from 'vitest-browser-svelte'
import { createQueryClient } from '@/shared/data/query'
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
		listAudit: mocks.listAudit,
		listRegistries: mocks.listRegistries,
		addRegistry: vi.fn(),
		updateRegistry: vi.fn(),
		removeRegistry: vi.fn(),
	},
}))

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
		mocks.listRegistries.mockResolvedValue([{ host: 'ghcr.io', username: 'cpu-commits' }])

		const screen = await render(SettingsView, { client: client() })

		await expect.element(screen.getByRole('heading', { name: 'Cloudflare' })).toBeVisible()
		await expect.element(screen.getByText('4 zonas visibles · cifrado')).toBeVisible()
		await expect.element(screen.getByLabelText('IPs permitidas para el panel')).toBeVisible()
		await expect.element(screen.getByText('ghcr.io')).toBeVisible()
		await expect.element(screen.getByText(/llega con la Fase/)).not.toBeInTheDocument()
	})
})
