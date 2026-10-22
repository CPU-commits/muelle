import { beforeEach, describe, expect, it, vi } from 'vitest'
import { render } from 'vitest-browser-svelte'
import { createQueryClient } from '@/shared/data/query'
import { m } from '@/shared/ui/i18n/paraglide/messages'
import { UnavailableError } from '@/shared/domain'
import type { SettingsSection, SettingsViewModel } from '..'
import type { Settings } from '../../../../domain'
import SettingsHarness from './SettingsHarness.svelte'
import { fakeSettings } from '../../../../domain/repositories/__fakes__/inMemoryAdminRepository'

const mocks = vi.hoisted(() => ({
	getSettings: vi.fn(),
	replaceState: vi.fn(),
	// Como SvelteKit: el callback corre cuando terminó la navegación, después de montar.
	afterNavigate: vi.fn((callback: () => void) => queueMicrotask(callback)),
}))

// `goto` lo importan otras secciones de la pantalla (el barrel la trae entera).
vi.mock('$app/navigation', () => ({
	afterNavigate: mocks.afterNavigate,
	goto: vi.fn(),
	replaceState: mocks.replaceState,
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
		setCloudflareOriginCa: vi.fn(),
		listAudit: vi.fn(),
		listRegistries: vi.fn(),
		addRegistry: vi.fn(),
		updateRegistry: vi.fn(),
		removeRegistry: vi.fn(),
	},
}))

const SETTINGS: Settings = fakeSettings({ require2fa: true })

async function mountViewModel(
	panelMoved = false,
	section: SettingsSection = 'proxy',
): Promise<SettingsViewModel> {
	const client = createQueryClient({ retry: false })
	let vm: SettingsViewModel | undefined
	await render(SettingsHarness, {
		client,
		section,
		panelMoved,
		onReady: (instance) => (vm = instance),
	})
	if (!vm) throw new Error('el harness no expuso el viewModel')
	return vm
}

describe('SettingsViewModel', () => {
	beforeEach(() => {
		vi.clearAllMocks()
		localStorage.clear()
		mocks.getSettings.mockResolvedValue(SETTINGS)
	})

	it('llegar con "Ir al panel" (?panel_moved=1) deja el aviso de la dirección nueva acá y saca el parámetro', async () => {
		// Lo de un aviso anterior de este origen no vale para este: acá no se sabe cuál era la
		// dirección anterior.
		localStorage.setItem('muelle:admin:panel-moved-from', 'https://panel.tudominio.cl')
		await mountViewModel(true)

		expect(localStorage.getItem('muelle:admin:panel-moved')).toBe(location.origin)
		expect(localStorage.getItem('muelle:admin:panel-moved-from')).toBeNull()
		await expect.poll(() => mocks.replaceState).toHaveBeenCalledWith('/settings/proxy', {})
	})

	it('sin el parámetro no toca el aviso ni la URL', async () => {
		await mountViewModel()

		expect(localStorage.getItem('muelle:admin:panel-moved')).toBeNull()
		expect(mocks.afterNavigate).not.toHaveBeenCalled()
	})

	it('carga los settings; mientras tanto, cargando', async () => {
		let resolve: (settings: Settings) => void = () => {}
		mocks.getSettings.mockReturnValue(new Promise<Settings>((done) => (resolve = done)))
		const vm = await mountViewModel()

		expect(vm.isLoading).toBe(true)
		expect(vm.settings).toBeNull()

		resolve(SETTINGS)
		await vi.waitUntil(() => !vm.isLoading)
		expect(vm.settings).toEqual(SETTINGS)
		expect(vm.loadError).toBeNull()
	})

	it('un error al cargar se muestra, y "Reintentar" vuelve a pedirlos', async () => {
		mocks.getSettings.mockRejectedValueOnce(new Error('boom'))
		const vm = await mountViewModel()
		await vi.waitUntil(() => !vm.isLoading)

		expect(vm.loadError).toBe(m.admin_settings_load_error())

		mocks.getSettings.mockResolvedValueOnce(SETTINGS)
		vm.retry()
		await vi.waitUntil(() => vm.settings !== null)
		expect(vm.loadError).toBeNull()
		expect(mocks.getSettings).toHaveBeenCalledTimes(2)
	})

	it('Registries, Usuarios y Auditoría no piden los settings: cada una trae lo suyo', async () => {
		for (const section of ['registries', 'users', 'audit'] as const) {
			const vm = await mountViewModel(false, section)
			expect(vm.section).toBe(section)
		}
		expect(mocks.getSettings).not.toHaveBeenCalled()

		await mountViewModel(false, 'security')
		await vi.waitUntil(() => mocks.getSettings.mock.calls.length > 0)
	})

	it('un backend caído dice que no se pudo contactar a muelle', async () => {
		mocks.getSettings.mockRejectedValueOnce(new UnavailableError('caído'))
		const vm = await mountViewModel()
		await vi.waitUntil(() => !vm.isLoading)

		expect(vm.loadError).toBe(m.common_unavailable())
	})
})
