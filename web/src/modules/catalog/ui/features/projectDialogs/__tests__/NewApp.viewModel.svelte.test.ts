import { beforeEach, describe, expect, it, vi } from 'vitest'
import { m } from '@/shared/ui/i18n/paraglide/messages'
import { ValidationError } from '@/shared/domain'
import { newServiceSpec, type ProjectSlug, SlugTaken, type StageSlug } from '../../../../domain'
import {
	fakeComposeApp,
	fakeServiceApp,
} from '../../../../domain/repositories/__fakes__/inMemoryCatalogRepository'
import { mountViewModel } from '../../../__tests__/mountViewModel'
import { COMPOSE_STARTER, NewAppViewModel } from '../NewApp.viewModel.svelte.ts'

const mocks = vi.hoisted(() => ({ createApp: vi.fn(), onDone: vi.fn(), goto: vi.fn() }))

vi.mock('@/modules/catalog/container', () => ({
	catalogUseCases: {
		listCatalog: vi.fn(),
		createProject: vi.fn(),
		renameProject: vi.fn(),
		deleteProject: vi.fn(),
		createStage: vi.fn(),
		deleteStage: vi.fn(),
		getApp: vi.fn(),
		createApp: mocks.createApp,
		updateApp: vi.fn(),
		deleteApp: vi.fn(),
	},
}))
vi.mock('$app/navigation', () => ({ goto: mocks.goto }))

const where = { project: 'auditfy' as ProjectSlug, stage: 'staging' as StageSlug }
const mount = () => mountViewModel(() => new NewAppViewModel(() => where, mocks.onDone))

describe('NewAppViewModel', () => {
	beforeEach(() => {
		vi.clearAllMocks()
		mocks.goto.mockResolvedValue(undefined)
	})

	it('una de formulario se crea con imagen y puerto y lleva a su Configuración', async () => {
		mocks.createApp.mockResolvedValue(fakeServiceApp('auditfy', 'staging', 'web'))
		const vm = await mount()
		expect(vm.kind).toBe('service')

		vm.form.setFieldValue('slug', 'web')
		vm.form.setFieldValue('image', ' nginx:1.27-alpine ')
		vm.form.setFieldValue('port', '80')
		await vm.form.handleSubmit()

		expect(mocks.createApp).toHaveBeenCalledWith({
			...where,
			slug: 'web',
			kind: 'service',
			spec: newServiceSpec('nginx:1.27-alpine', 80),
		})
		expect(mocks.onDone).toHaveBeenCalled()
		expect(mocks.goto).toHaveBeenCalledWith(
			'/projects/auditfy/stages/staging/apps/web/configuracion',
		)
	})

	it('una compose arranca con un YAML de ejemplo y no pide imagen ni puerto', async () => {
		mocks.createApp.mockResolvedValue(fakeComposeApp('auditfy', 'staging', 'jobs'))
		const vm = await mount()

		vm.setKind('compose')
		expect(vm.kind).toBe('compose')
		expect(vm.kindHint).toBe(m.catalog_app_kind_compose_hint())
		vm.form.setFieldValue('slug', 'jobs')
		await vm.form.handleSubmit()

		expect(mocks.createApp).toHaveBeenCalledWith({
			...where,
			slug: 'jobs',
			kind: 'compose',
			composeYaml: COMPOSE_STARTER,
		})
	})

	it('valida antes de mandar: imagen y puerto con las reglas del spec', async () => {
		const vm = await mount()

		vm.form.setFieldValue('slug', 'web')
		vm.form.setFieldValue('image', 'Nginx:latest')
		vm.form.setFieldValue('port', '0')
		await vm.form.handleSubmit()

		expect(mocks.createApp).not.toHaveBeenCalled()
		expect(vm.form.getFieldMeta('image')?.errors).toContain(m.catalog_spec_image_invalid())
		expect(vm.form.getFieldMeta('port')?.errors).toContain(m.catalog_spec_port())
	})

	it('los errores del backend van al campo, traducidos; editar el campo los limpia', async () => {
		const vm = await mount()
		vm.form.setFieldValue('slug', 'api')
		vm.form.setFieldValue('image', 'nginx')
		vm.form.setFieldValue('port', '80')

		mocks.createApp.mockRejectedValueOnce(new SlugTaken('app'))
		await vm.form.handleSubmit()
		expect(vm.errors.fields).toEqual({ slug: m.catalog_error_app_taken() })
		vm.edited('slug')
		expect(vm.errors.fields).toEqual({})

		mocks.createApp.mockRejectedValueOnce(
			new ValidationError('la imagen no es una referencia válida de Docker', 'image'),
		)
		await vm.form.handleSubmit()
		expect(vm.errors.fields).toEqual({ image: m.catalog_spec_image_invalid() })
		expect(mocks.goto).not.toHaveBeenCalled()
	})
})
