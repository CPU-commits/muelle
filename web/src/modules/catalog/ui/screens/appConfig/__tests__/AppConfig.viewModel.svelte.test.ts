import { beforeEach, describe, expect, it, vi } from 'vitest'
import { m } from '@/shared/ui/i18n/paraglide/messages'
import { UnavailableError } from '@/shared/domain'
import { AppNotFound, appRefFrom, appSummaryOf } from '../../../../domain'
import { fakeServiceApp } from '../../../../domain/repositories/__fakes__/inMemoryCatalogRepository'
import { mountViewModel, until } from '../../../__tests__/mountViewModel'
import { AppConfigViewModel } from '../AppConfig.viewModel.svelte.ts'

const mocks = vi.hoisted(() => ({ getApp: vi.fn() }))

vi.mock('@/modules/catalog/container', () => ({
	catalogUseCases: {
		listCatalog: vi.fn(),
		createProject: vi.fn(),
		renameProject: vi.fn(),
		deleteProject: vi.fn(),
		createStage: vi.fn(),
		deleteStage: vi.fn(),
		getApp: mocks.getApp,
		createApp: vi.fn(),
		updateApp: vi.fn(),
		deleteApp: vi.fn(),
	},
}))

const ref = appRefFrom('auditfy', 'staging', 'api')
const mount = () => mountViewModel(() => new AppConfigViewModel(() => ref))

describe('AppConfigViewModel', () => {
	beforeEach(() => {
		vi.clearAllMocks()
	})

	it('carga la app y arma su resumen', async () => {
		const app = fakeServiceApp('auditfy', 'staging', 'api')
		mocks.getApp.mockResolvedValue(app)
		const vm = await mount()

		await until(() => !vm.isLoading)

		expect(mocks.getApp).toHaveBeenCalledWith(ref, expect.anything())
		expect(vm.app).toBe(app)
		expect(vm.summary).toEqual(appSummaryOf(app))
		expect(vm.key).toBe('auditfy/staging/api')
		expect(vm.isNotFound).toBe(false)
		expect(vm.loadError).toBeNull()
	})

	it('una app que no existe no es un error para reintentar', async () => {
		mocks.getApp.mockRejectedValue(new AppNotFound())
		const vm = await mount()

		await until(() => !vm.isLoading)

		expect(vm.isNotFound).toBe(true)
		expect(vm.loadError).toBeNull()
	})

	it('un backend caído se muestra con su texto y se puede reintentar', async () => {
		mocks.getApp.mockRejectedValue(new UnavailableError('caído'))
		const vm = await mount()
		await until(() => !vm.isLoading)

		expect(vm.loadError).toBe(m.common_unavailable())

		mocks.getApp.mockResolvedValue(fakeServiceApp('auditfy', 'staging', 'api'))
		vm.retry()
		await until(() => vm.app !== undefined)
		expect(vm.loadError).toBeNull()
	})
})
