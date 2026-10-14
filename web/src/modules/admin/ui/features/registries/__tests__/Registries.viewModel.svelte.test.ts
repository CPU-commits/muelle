import { onlineManager, type QueryClient } from '@tanstack/svelte-query'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { render } from 'vitest-browser-svelte'
import { createQueryClient } from '@/shared/data/query'
import { m } from '@/shared/ui/i18n/paraglide/messages'
import { formatDateTime, formatRelative } from '@/shared/ui'
import { toast } from '@/shared/ui/stores'
import { UnavailableError } from '@/shared/domain'
import type { RegistriesViewModel } from '..'
import { RegistryExists, RegistryNotFound } from '../../../../domain'
import RegistriesHarness from './RegistriesHarness.svelte'
import { cacheHolds } from '../../../__tests__/cacheHolds'

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
		removeCloudflareToken: vi.fn(),
		listAudit: vi.fn(),
		listRegistries: mocks.listRegistries,
		addRegistry: mocks.addRegistry,
		updateRegistry: mocks.updateRegistry,
		removeRegistry: mocks.removeRegistry,
	},
}))

let client: QueryClient

const CREATED = new Date('2026-10-01T12:00:00.000Z')
const UPDATED = new Date('2026-10-09T15:30:00.000Z')

function registry(host: string, username: string) {
	return { host, username, createdAt: CREATED, updatedAt: UPDATED }
}

async function mountViewModel(): Promise<RegistriesViewModel> {
	client = createQueryClient({ retry: false })
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
		mocks.listRegistries.mockResolvedValue([registry('ghcr.io', 'cpu-commits')])
		mocks.addRegistry.mockResolvedValue(registry('quay.io', 'bot'))
		mocks.updateRegistry.mockResolvedValue(registry('ghcr.io', 'bot'))
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
		expect(await cacheHolds(client, 'secreto')).toBe(false)
	})

	it('la contraseña no queda en el cache ni en el campo, aunque guardar falle', async () => {
		mocks.updateRegistry.mockRejectedValueOnce(new Error('boom'))
		const vm = await mountViewModel()
		const registry = vm.registries[0]
		if (!registry) throw new Error('falta el registry')

		vm.openEdit(registry)
		vm.setField('password', 'clave-nueva')
		await vm.submit()

		expect(vm.dialog).toEqual({ mode: 'edit', host: 'ghcr.io' })
		expect(vm.password).toBe('')
		expect(await cacheHolds(client, 'clave-nueva')).toBe(false)
	})

	it('un error que se ve sin mandar nada no borra la contraseña escrita', async () => {
		const vm = await mountViewModel()
		vm.openAdd()
		vm.setField('host', 'ghcr.io/org')
		vm.setField('username', 'bot')
		vm.setField('password', 'clave-escrita')

		await vm.submit()

		expect(vm.errors).toEqual({ host: m.admin_error_registry_host_path() })
		expect(mocks.addRegistry).not.toHaveBeenCalled()
		expect(vm.password).toBe('clave-escrita')

		vm.setField('host', 'quay.io')
		vm.setField('username', ' ')
		await vm.submit()
		expect(vm.errors).toEqual({ username: m.common_required() })
		expect(vm.password).toBe('clave-escrita')
	})

	it('el host dice qué sobra: el esquema, la ruta, o que no es un host', async () => {
		const vm = await mountViewModel()
		vm.openAdd()
		vm.setField('username', 'bot')
		vm.setField('password', 'x')
		const errorFor = async (host: string) => {
			vm.setField('host', host)
			await vm.submit()
			return vm.errors.host
		}

		expect(await errorFor('https://ghcr.io')).toBe(m.admin_error_registry_host_scheme())
		expect(await errorFor('ghcr.io/')).toBe(m.admin_error_registry_host_path())
		expect(await errorFor('mi registry')).toBe(m.admin_error_registry_host_format())
		expect(await errorFor('')).toBe(m.common_required())
		expect(mocks.addRegistry).not.toHaveBeenCalled()
	})

	it('el usuario y la contraseña se validan con los límites del backend', async () => {
		const vm = await mountViewModel()
		vm.openAdd()
		vm.setField('host', 'quay.io')
		vm.setField('username', 'a'.repeat(256))
		vm.setField('password', 'x')

		await vm.submit()

		expect(vm.errors).toEqual({ username: m.admin_error_registry_username_long({ max: 255 }) })
		expect(mocks.addRegistry).not.toHaveBeenCalled()
		expect(vm.password).toBe('x')
	})

	it('cada fila dice cuándo se cambiaron sus credenciales', async () => {
		const vm = await mountViewModel()
		const [first] = vm.registries
		if (!first) throw new Error('falta el registry')

		expect(vm.updatedLabel(first)).toBe(
			m.admin_registries_updated({ when: formatRelative(UPDATED) }),
		)
		expect(vm.updatedTitle(first)).toBe(formatDateTime(UPDATED))
	})

	it('un host repetido (lo dice el backend) va al campo host', async () => {
		const vm = await mountViewModel()
		vm.openAdd()
		vm.setField('host', 'ghcr.io')
		vm.setField('username', 'bot')
		vm.setField('password', 'x')

		mocks.addRegistry.mockRejectedValueOnce(new RegistryExists())
		await vm.submit()

		expect(vm.errors).toEqual({ host: m.admin_error_registry_exists() })
		expect(vm.dialog).toEqual({ mode: 'add' })
		// Ya salió: la contraseña no se guarda en ningún lado.
		expect(vm.password).toBe('')
	})

	it('sin red, guardar falla enseguida en vez de quedar pausado con la contraseña', async () => {
		mocks.addRegistry.mockRejectedValueOnce(new UnavailableError('sin red'))
		const vm = await mountViewModel()
		onlineManager.setOnline(false)
		try {
			vm.openAdd()
			vm.setField('host', 'quay.io')
			vm.setField('username', 'bot')
			vm.setField('password', 'clave-sin-red')
			await vm.submit()

			expect(mocks.addRegistry).toHaveBeenCalled()
			expect(await cacheHolds(client, 'clave-sin-red')).toBe(false)
		} finally {
			onlineManager.setOnline(true)
		}
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

	it('un error al eliminar queda adentro de la confirmación; si ya no estaba, avisa', async () => {
		const info = vi.spyOn(toast, 'info')
		const vm = await mountViewModel()

		mocks.removeRegistry.mockRejectedValueOnce(new UnavailableError('caído'))
		vm.askRemove('ghcr.io')
		await vm.confirmRemove()
		expect(vm.removing).toBe('ghcr.io')
		expect(vm.removeError).toBe(m.common_unavailable())

		mocks.removeRegistry.mockRejectedValueOnce(new RegistryNotFound())
		await vm.confirmRemove()
		expect(vm.removing).toBeNull()
		expect(info).toHaveBeenCalledWith(m.admin_error_registry_not_found())
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
