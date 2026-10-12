import { QueryClient } from '@tanstack/svelte-query'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { render } from 'vitest-browser-svelte'
import { m } from '@/shared/ui/i18n/paraglide/messages'
import { toast } from '@/shared/ui/stores'
import { UnavailableError } from '@/shared/domain'
import type { RegistriesViewModel } from '..'
import { InvalidRegistryHost, RegistryExists } from '../../../../domain'
import RegistriesHarness from './RegistriesHarness.svelte'

const mocks = vi.hoisted(() => ({
	listRegistries: vi.fn(),
	addRegistry: vi.fn(),
	updateRegistry: vi.fn(),
	removeRegistry: vi.fn(),
}))

vi.mock('@/modules/admin/container', () => ({
	adminUseCases: {
		getSettings: vi.fn(),
		updateSecurity: vi.fn(),
		updatePublicIp: vi.fn(),
		updateAcmeEmail: vi.fn(),
		updateProxyPorts: vi.fn(),
		replaceCloudflareToken: vi.fn(),
		listAudit: vi.fn(),
		listRegistries: mocks.listRegistries,
		addRegistry: mocks.addRegistry,
		updateRegistry: mocks.updateRegistry,
		removeRegistry: mocks.removeRegistry,
	},
}))

async function mountViewModel(): Promise<RegistriesViewModel> {
	const client = new QueryClient({
		defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
	})
	let vm: RegistriesViewModel | undefined
	await render(RegistriesHarness, { client, onReady: (instance) => (vm = instance) })
	if (!vm) throw new Error('el harness no expuso el viewModel')
	await vi.waitUntil(() => !vm?.isLoading)
	return vm
}

describe('RegistriesViewModel', () => {
	beforeEach(() => {
		vi.clearAllMocks()
		vi.restoreAllMocks()
		mocks.listRegistries.mockResolvedValue([{ host: 'ghcr.io', username: 'cpu-commits' }])
		mocks.addRegistry.mockResolvedValue({ host: 'quay.io', username: 'bot' })
		mocks.updateRegistry.mockResolvedValue({ host: 'ghcr.io', username: 'bot' })
		mocks.removeRegistry.mockResolvedValue(undefined)
	})

	it('contra un backend sin la Fase 2 muestra "llega con la Fase 2", no un error', async () => {
		mocks.listRegistries.mockRejectedValue(new UnavailableError('x', { phase: 2 }))
		const vm = await mountViewModel()

		expect(vm.pendingPhase).toBe(2)
		expect(vm.loadError).toBeNull()
	})

	it('un backend caído sí es un error', async () => {
		mocks.listRegistries.mockRejectedValue(new UnavailableError('caído'))
		const vm = await mountViewModel()

		expect(vm.pendingPhase).toBeNull()
		expect(vm.loadError).toBe(m.common_unavailable())
	})

	it('agrega uno nuevo con lo escrito y cierra el modal', async () => {
		const success = vi.spyOn(toast, 'success')
		const vm = await mountViewModel()

		vm.openAdd()
		vm.setField('host', 'quay.io')
		vm.setField('username', 'bot')
		vm.setField('password', 'secreto')
		await vm.submit()

		expect(mocks.addRegistry).toHaveBeenCalledWith({
			host: 'quay.io',
			username: 'bot',
			password: 'secreto',
		})
		expect(vm.dialog).toBeNull()
		expect(vm.password).toBe('')
		expect(success).toHaveBeenCalledWith(m.admin_registry_added())
	})

	it('un host inválido o repetido va al campo host', async () => {
		const vm = await mountViewModel()
		vm.openAdd()

		mocks.addRegistry.mockRejectedValueOnce(new InvalidRegistryHost())
		await vm.submit()
		expect(vm.errors).toEqual({ host: m.admin_error_registry_host() })

		mocks.addRegistry.mockRejectedValueOnce(new RegistryExists())
		await vm.submit()
		expect(vm.errors).toEqual({ host: m.admin_error_registry_exists() })
		expect(vm.dialog).toEqual({ mode: 'add' })
	})

	it('cambiar credenciales no cambia el host y pide la contraseña de nuevo', async () => {
		const vm = await mountViewModel()
		const registry = vm.registries[0]
		if (!registry) throw new Error('falta el registry')

		vm.openEdit(registry)
		expect(vm.username).toBe('cpu-commits')
		expect(vm.password).toBe('')
		vm.setField('host', 'otro.io')
		vm.setField('password', 'nueva')
		await vm.submit()

		expect(mocks.updateRegistry).toHaveBeenCalledWith({
			host: 'ghcr.io',
			username: 'cpu-commits',
			password: 'nueva',
		})
	})

	it('eliminar pide confirmación', async () => {
		const vm = await mountViewModel()

		vm.askRemove('ghcr.io')
		expect(mocks.removeRegistry).not.toHaveBeenCalled()
		await vm.confirmRemove()

		expect(mocks.removeRegistry).toHaveBeenCalledWith('ghcr.io')
		expect(vm.removing).toBeNull()
	})
})
