import { beforeEach, describe, expect, it, vi } from 'vitest'
import { m } from '@/shared/ui/i18n/paraglide/messages'
import { ValidationError } from '@/shared/domain'
import { PROJECT_NAME_MAX } from '../../../../domain'
import { fakeProject } from '../../../../domain/repositories/__fakes__/inMemoryCatalogRepository'
import { mountViewModel } from '../../../__tests__/mountViewModel'
import { RenameProjectViewModel } from '../RenameProject.viewModel.svelte.ts'

const mocks = vi.hoisted(() => ({ renameProject: vi.fn(), onDone: vi.fn() }))

vi.mock('@/modules/catalog/container', () => ({
	catalogUseCases: {
		listCatalog: vi.fn(),
		createProject: vi.fn(),
		renameProject: mocks.renameProject,
		deleteProject: vi.fn(),
		createStage: vi.fn(),
		deleteStage: vi.fn(),
		getApp: vi.fn(),
		createApp: vi.fn(),
		updateApp: vi.fn(),
		deleteApp: vi.fn(),
	},
}))

const project = fakeProject('auditfy', 'Auditfy')
const mount = () => mountViewModel(() => new RenameProjectViewModel(() => project, mocks.onDone))

describe('RenameProjectViewModel', () => {
	beforeEach(() => {
		vi.clearAllMocks()
		mocks.renameProject.mockResolvedValue({ ...project, name: 'Auditfy SA' })
	})

	it('arranca con el nombre actual y renombra', async () => {
		const vm = await mount()
		expect(vm.form.state.values.name).toBe('Auditfy')

		vm.form.setFieldValue('name', 'Auditfy SA')
		await vm.form.handleSubmit()

		expect(mocks.renameProject).toHaveBeenCalledWith({ project: 'auditfy', name: 'Auditfy SA' })
		expect(mocks.onDone).toHaveBeenCalled()
	})

	it('sin cambios (o solo espacios) cierra sin hacer el PATCH', async () => {
		const vm = await mount()

		vm.form.setFieldValue('name', '  Auditfy ')
		await vm.form.handleSubmit()

		expect(mocks.renameProject).not.toHaveBeenCalled()
		expect(mocks.onDone).toHaveBeenCalled()
	})

	it('un nombre vacío no se manda y un 422 del backend va al campo, traducido', async () => {
		const vm = await mount()

		vm.form.setFieldValue('name', '   ')
		await vm.form.handleSubmit()
		expect(mocks.renameProject).not.toHaveBeenCalled()
		expect(vm.form.getFieldMeta('name')?.errors).toContain(m.common_required())

		mocks.renameProject.mockRejectedValueOnce(
			new ValidationError('no puede superar 64 caracteres', 'name'),
		)
		vm.form.setFieldValue('name', 'Otro nombre')
		await vm.form.handleSubmit()
		expect(vm.errors.fields).toEqual({
			name: m.catalog_error_name_long({ max: PROJECT_NAME_MAX }),
		})
		expect(mocks.onDone).not.toHaveBeenCalled()
	})
})
