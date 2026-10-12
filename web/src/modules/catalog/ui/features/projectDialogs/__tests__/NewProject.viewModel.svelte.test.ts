import { beforeEach, describe, expect, it, vi } from 'vitest'
import { m } from '@/shared/ui/i18n/paraglide/messages'
import { ValidationError } from '@/shared/domain'
import { SlugTaken } from '../../../../domain'
import { fakeProject } from '../../../../domain/repositories/__fakes__/inMemoryCatalogRepository'
import { mountViewModel } from '../../../__tests__/mountViewModel'
import { NewProjectViewModel } from '../NewProject.viewModel.svelte.ts'

const mocks = vi.hoisted(() => ({ createProject: vi.fn(), onDone: vi.fn() }))

vi.mock('@/modules/catalog/container', () => ({
	catalogUseCases: {
		listCatalog: vi.fn(),
		createProject: mocks.createProject,
		renameProject: vi.fn(),
		deleteProject: vi.fn(),
		createStage: vi.fn(),
		deleteStage: vi.fn(),
		getApp: vi.fn(),
		createApp: vi.fn(),
		updateApp: vi.fn(),
		deleteApp: vi.fn(),
	},
}))

const mount = () => mountViewModel(() => new NewProjectViewModel(mocks.onDone))

describe('NewProjectViewModel', () => {
	beforeEach(() => {
		vi.clearAllMocks()
		mocks.createProject.mockResolvedValue(fakeProject('aumi-agents', 'Aumi Agents'))
	})

	it('sugiere el slug desde el nombre hasta que se edita a mano', async () => {
		const vm = await mount()

		vm.form.setFieldValue('name', 'Aumi Agents')
		vm.nameEdited('Aumi Agents')
		expect(vm.form.state.values.slug).toBe('aumi-agents')

		vm.form.setFieldValue('slug', 'agents')
		vm.slugEdited()
		vm.nameEdited('Otro nombre')
		expect(vm.form.state.values.slug).toBe('agents')
	})

	it('crea el proyecto y cierra el diálogo', async () => {
		const vm = await mount()
		vm.form.setFieldValue('name', 'Aumi Agents')
		vm.form.setFieldValue('slug', 'aumi-agents')

		await vm.form.handleSubmit()

		expect(mocks.createProject).toHaveBeenCalledWith({
			name: 'Aumi Agents',
			slug: 'aumi-agents',
		})
		expect(mocks.onDone).toHaveBeenCalled()
	})

	it('el slug tomado y un 422 van a su campo; editar el campo lo limpia', async () => {
		mocks.createProject.mockRejectedValueOnce(new SlugTaken('project'))
		const vm = await mount()
		vm.form.setFieldValue('name', 'Auditfy')
		vm.form.setFieldValue('slug', 'auditfy')

		await vm.form.handleSubmit()
		expect(vm.errors.fields).toEqual({ slug: m.catalog_error_project_taken() })
		expect(mocks.onDone).not.toHaveBeenCalled()

		vm.slugEdited()
		expect(vm.errors.fields).toEqual({})

		mocks.createProject.mockRejectedValueOnce(
			new ValidationError('no puede superar 64', 'name'),
		)
		await vm.form.handleSubmit()
		expect(vm.errors.fields).toEqual({ name: 'no puede superar 64' })
	})
})
