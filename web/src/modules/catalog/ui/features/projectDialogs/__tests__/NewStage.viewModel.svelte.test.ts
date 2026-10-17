import { beforeEach, describe, expect, it, vi } from 'vitest'
import { m } from '@/shared/ui/i18n/paraglide/messages'
import { toast } from '@/shared/ui/stores'
import {
	InvalidSlug,
	type ProjectSlug,
	ProjectNotFound,
	SlugTaken,
	STAGE_SLUG_MAX,
	StageNetworkConflict,
} from '../../../../domain'
import { fakeStage } from '../../../../domain/repositories/__fakes__/inMemoryCatalogRepository'
import { mountViewModel } from '../../../__tests__/mountViewModel'
import { NewStageViewModel } from '../NewStage.viewModel.svelte.ts'

const mocks = vi.hoisted(() => ({ createStage: vi.fn(), onDone: vi.fn() }))

vi.mock('@/modules/catalog/container', () => ({
	catalogUseCases: {
		listCatalog: vi.fn(),
		createProject: vi.fn(),
		renameProject: vi.fn(),
		deleteProject: vi.fn(),
		createStage: mocks.createStage,
		deleteStage: vi.fn(),
		getApp: vi.fn(),
		createApp: vi.fn(),
		updateApp: vi.fn(),
		deleteApp: vi.fn(),
	},
}))

const mount = () =>
	mountViewModel(() => new NewStageViewModel(() => 'auditfy' as ProjectSlug, mocks.onDone))

describe('NewStageViewModel', () => {
	beforeEach(() => {
		vi.clearAllMocks()
		mocks.createStage.mockResolvedValue(fakeStage('auditfy', 'qa'))
	})

	it('crea el stage en el proyecto, avisa y cierra', async () => {
		const success = vi.spyOn(toast, 'success')
		const vm = await mount()

		vm.form.setFieldValue('slug', 'qa')
		await vm.form.handleSubmit()

		expect(mocks.createStage).toHaveBeenCalledWith({ project: 'auditfy', slug: 'qa' })
		expect(success).toHaveBeenCalledWith(m.catalog_stage_created({ stage: 'qa' }))
		expect(mocks.onDone).toHaveBeenCalled()
	})

	it('un slug que no cumple la regla no se manda', async () => {
		const vm = await mount()

		vm.form.setFieldValue('slug', 'a-very-long-stage')
		await vm.form.handleSubmit()

		expect(mocks.createStage).not.toHaveBeenCalled()
		expect(vm.form.getFieldMeta('slug')?.errors).toContain(
			m.catalog_error_slug_format({ max: STAGE_SLUG_MAX }),
		)
	})

	it('el slug repetido y el del dominio van al campo; el resto, arriba', async () => {
		const vm = await mount()
		vm.form.setFieldValue('slug', 'staging')

		mocks.createStage.mockRejectedValueOnce(new SlugTaken('stage'))
		await vm.form.handleSubmit()
		expect(vm.errors).toEqual({
			fields: { slug: m.catalog_error_stage_taken() },
			form: null,
		})

		mocks.createStage.mockRejectedValueOnce(
			new InvalidSlug('format', STAGE_SLUG_MAX, 'staging'),
		)
		await vm.form.handleSubmit()
		expect(vm.errors.fields).toEqual({
			slug: m.catalog_error_slug_format({ max: STAGE_SLUG_MAX }),
		})

		mocks.createStage.mockRejectedValueOnce(new ProjectNotFound())
		await vm.form.handleSubmit()
		expect(vm.errors).toEqual({ fields: {}, form: m.catalog_error_project_not_found() })
		expect(mocks.onDone).not.toHaveBeenCalled()
	})

	it('una red de Docker con el nombre del stage va arriba, nombrando la red', async () => {
		const vm = await mount()
		vm.form.setFieldValue('slug', 'qa')

		mocks.createStage.mockRejectedValueOnce(new StageNetworkConflict('auditfy_qa'))
		await vm.form.handleSubmit()

		expect(vm.errors).toEqual({
			fields: {},
			form: m.catalog_error_stage_network_conflict({ network: 'auditfy_qa' }),
		})
		expect(mocks.onDone).not.toHaveBeenCalled()
	})
})
