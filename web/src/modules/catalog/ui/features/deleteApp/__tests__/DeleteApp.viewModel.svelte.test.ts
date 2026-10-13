import { beforeEach, describe, expect, it, vi } from 'vitest'
import { m } from '@/shared/ui/i18n/paraglide/messages'
import { toast } from '@/shared/ui/stores'
import { UnavailableError } from '@/shared/domain'
import { AppNotFound, appRefFrom } from '../../../../domain'
import { mountViewModel } from '../../../__tests__/mountViewModel'
import { DeleteAppViewModel } from '../DeleteApp.viewModel.svelte.ts'

const mocks = vi.hoisted(() => ({ deleteApp: vi.fn(), goto: vi.fn() }))

vi.mock('@/modules/catalog/container', () => ({
	catalogUseCases: {
		listCatalog: vi.fn(),
		createProject: vi.fn(),
		renameProject: vi.fn(),
		deleteProject: vi.fn(),
		createStage: vi.fn(),
		deleteStage: vi.fn(),
		getApp: vi.fn(),
		createApp: vi.fn(),
		updateApp: vi.fn(),
		deleteApp: mocks.deleteApp,
	},
}))
vi.mock('$app/navigation', () => ({ goto: mocks.goto }))

const ref = appRefFrom('auditfy', 'staging', 'api')
const mount = () => mountViewModel(() => new DeleteAppViewModel(() => ref))

describe('DeleteAppViewModel', () => {
	beforeEach(() => {
		vi.clearAllMocks()
		mocks.deleteApp.mockResolvedValue(undefined)
		mocks.goto.mockResolvedValue(undefined)
	})

	it('nombra la app y su stack', async () => {
		const vm = await mount()

		expect(vm.app).toBe('api')
		expect(vm.stackName).toBe('auditfy_staging_api')
	})

	it('confirmar borra, avisa y vuelve a Proyectos', async () => {
		const success = vi.spyOn(toast, 'success')
		const vm = await mount()

		vm.ask()
		expect(vm.confirming).toBe(true)
		await vm.confirm()

		expect(mocks.deleteApp).toHaveBeenCalledWith(ref)
		expect(success).toHaveBeenCalledWith(m.catalog_app_deleted({ app: 'api' }))
		expect(vm.confirming).toBe(false)
		expect(mocks.goto).toHaveBeenCalledWith('/')
	})

	it('si ya no existía, el resultado es el mismo', async () => {
		mocks.deleteApp.mockRejectedValue(new AppNotFound())
		const vm = await mount()

		vm.ask()
		await vm.confirm()

		expect(mocks.goto).toHaveBeenCalledWith('/')
	})

	it('otro error se queda en la pantalla con el motivo', async () => {
		mocks.deleteApp.mockRejectedValue(new UnavailableError('caído'))
		const error = vi.spyOn(toast, 'error')
		const vm = await mount()

		vm.ask()
		await vm.confirm()

		expect(error).toHaveBeenCalledWith(m.catalog_delete_app_error(), m.common_unavailable())
		expect(vm.confirming).toBe(false)
		expect(mocks.goto).not.toHaveBeenCalled()
	})

	it('cancelar cierra el diálogo', async () => {
		const vm = await mount()

		vm.ask()
		vm.cancel()

		expect(vm.confirming).toBe(false)
		expect(mocks.deleteApp).not.toHaveBeenCalled()
	})
})
