import { beforeEach, describe, expect, it, vi } from 'vitest'
import { render } from 'vitest-browser-svelte'
import { createQueryClient } from '@/shared/data/query'
import { m } from '@/shared/ui/i18n/paraglide/messages'
import { UnavailableError } from '@/shared/domain'
import type { SettingsViewModel } from '..'
import type { Settings } from '../../../../domain'
import SettingsHarness from './SettingsHarness.svelte'

const mocks = vi.hoisted(() => ({ getSettings: vi.fn() }))

vi.mock('@/modules/admin/container', () => ({
	adminUseCases: {
		getSettings: mocks.getSettings,
		updateSecurity: vi.fn(),
		updatePublicIp: vi.fn(),
		updateAcmeEmail: vi.fn(),
		updateProxyPorts: vi.fn(),
		replaceCloudflareToken: vi.fn(),
		removeCloudflareToken: vi.fn(),
		listAudit: vi.fn(),
		listRegistries: vi.fn(),
		addRegistry: vi.fn(),
		updateRegistry: vi.fn(),
		removeRegistry: vi.fn(),
	},
}))

const PHASE_1: Settings = { require2fa: true, proxy: null, panelAccess: null }

async function mountViewModel(): Promise<SettingsViewModel> {
	const client = createQueryClient({ retry: false })
	let vm: SettingsViewModel | undefined
	await render(SettingsHarness, { client, onReady: (instance) => (vm = instance) })
	if (!vm) throw new Error('el harness no expuso el viewModel')
	return vm
}

describe('SettingsViewModel', () => {
	beforeEach(() => {
		vi.clearAllMocks()
	})

	it('carga los settings; mientras tanto, cargando', async () => {
		let resolve: (settings: Settings) => void = () => {}
		mocks.getSettings.mockReturnValue(new Promise<Settings>((done) => (resolve = done)))
		const vm = await mountViewModel()

		expect(vm.isLoading).toBe(true)
		expect(vm.settings).toBeNull()

		resolve(PHASE_1)
		await vi.waitUntil(() => !vm.isLoading)
		expect(vm.settings).toEqual(PHASE_1)
		expect(vm.loadError).toBeNull()
		expect(vm.proxyPhase).toBe(3)
	})

	it('un error al cargar se muestra, y "Reintentar" vuelve a pedirlos', async () => {
		mocks.getSettings.mockRejectedValueOnce(new Error('boom'))
		const vm = await mountViewModel()
		await vi.waitUntil(() => !vm.isLoading)

		expect(vm.loadError).toBe(m.admin_settings_load_error())

		mocks.getSettings.mockResolvedValueOnce(PHASE_1)
		vm.retry()
		await vi.waitUntil(() => vm.settings !== null)
		expect(vm.loadError).toBeNull()
		expect(mocks.getSettings).toHaveBeenCalledTimes(2)
	})

	it('un backend caído dice que no se pudo contactar a muelle', async () => {
		mocks.getSettings.mockRejectedValueOnce(new UnavailableError('caído'))
		const vm = await mountViewModel()
		await vi.waitUntil(() => !vm.isLoading)

		expect(vm.loadError).toBe(m.common_unavailable())
	})
})
