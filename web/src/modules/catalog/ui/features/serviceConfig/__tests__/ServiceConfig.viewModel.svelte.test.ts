import type { BeforeNavigate } from '@sveltejs/kit'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createQueryClient } from '@/shared/data/query'
import { m } from '@/shared/ui/i18n/paraglide/messages'
import { toast } from '@/shared/ui/stores'
import { ValidationError } from '@/shared/domain'
import { catalogKeys } from '../../../../data'
import {
	appRefFrom,
	DEFAULT_HEALTHCHECK_TIMINGS,
	KindMismatch,
	type ServiceApp,
} from '../../../../domain'
import { fakeServiceApp } from '../../../../domain/repositories/__fakes__/inMemoryCatalogRepository'
import { mountViewModel } from '../../../__tests__/mountViewModel'
import { ServiceConfigViewModel } from '../ServiceConfig.viewModel.svelte.ts'

const mocks = vi.hoisted(() => ({
	updateApp: vi.fn(),
	guards: [] as ((navigation: BeforeNavigate) => void)[],
}))

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
		updateApp: mocks.updateApp,
		deleteApp: vi.fn(),
	},
}))

// La subnav lee el hash de la URL; fuera de SvelteKit no hay `page` ni `beforeNavigate`.
vi.mock('$app/state', () => ({ page: { url: new URL('http://localhost/configuracion') } }))
vi.mock('$app/navigation', () => ({
	beforeNavigate: (callback: (navigation: BeforeNavigate) => void) => {
		mocks.guards.push(callback)
	},
	goto: vi.fn(),
}))

function linkNavigation(): BeforeNavigate {
	return { type: 'link', willUnload: false, cancel: vi.fn() } as unknown as BeforeNavigate
}

const app: ServiceApp = fakeServiceApp('auditfy', 'staging', 'api', {
	spec: {
		image: 'ghcr.io/aumi-app/backend-app:sha-3f9c2e1',
		port: 3000,
		replicas: 2,
		command: [],
		healthcheck: { type: 'http', path: '/health', ...DEFAULT_HEALTHCHECK_TIMINGS },
		volumes: [{ name: 'uploads', mount: '/app/uploads' }],
		resources: { memoryMb: 512, cpus: 1 },
	},
})
const ref = appRefFrom('auditfy', 'staging', 'api')

const mount = (client = createQueryClient({ retry: false })) =>
	mountViewModel(() => new ServiceConfigViewModel(() => app), client)

describe('ServiceConfigViewModel', () => {
	beforeEach(() => {
		vi.clearAllMocks()
		mocks.guards.length = 0
		mocks.updateApp.mockImplementation(async ({ changes }) => ({
			...app,
			...(changes.spec ? { spec: changes.spec } : {}),
		}))
	})

	it('guardar la imagen manda el spec completo con la imagen nueva', async () => {
		const vm = await mount()

		vm.image.form.setFieldValue('image', ' nginx:1.28 ')
		await vm.image.form.handleSubmit()

		expect(mocks.updateApp).toHaveBeenCalledWith({
			ref,
			changes: { spec: { ...app.spec, image: 'nginx:1.28' } },
		})
		expect(vm.image.form.state.values.image).toBe('nginx:1.28')
	})

	it('sin cambios no llama al backend y lo dice', async () => {
		const info = vi.spyOn(toast, 'info')
		const vm = await mount()

		await vm.run.form.handleSubmit()

		expect(mocks.updateApp).not.toHaveBeenCalled()
		expect(info).toHaveBeenCalledWith(m.catalog_config_no_changes())
	})

	it('valida con las reglas del backend antes de mandar, y el error queda en su campo', async () => {
		const vm = await mount()

		vm.run.form.setFieldValue('port', '0')
		await vm.run.form.handleSubmit()

		expect(mocks.updateApp).not.toHaveBeenCalled()
		expect(vm.run.form.getFieldMeta('port')?.errors).toContain(m.catalog_spec_port())
	})

	it('un 422 del backend va al campo, traducido, hasta que se edita', async () => {
		mocks.updateApp.mockRejectedValue(
			new ValidationError(
				'debe empezar con / y usar solo letras, números y ._~/?=%+-',
				'healthcheck.path',
			),
		)
		const vm = await mount()

		vm.health.form.setFieldValue('path', '/otro')
		await vm.health.form.handleSubmit()

		expect(vm.health.saver.serverError('path', '/otro')).toBe(m.catalog_spec_health_path())
		expect(vm.health.saver.serverError('path', '/otro-mas')).toBeNull()
		expect(vm.health.saver.formError).toBeNull()
	})

	it('"Ninguno" avisa que no hay cero downtime y no tiene vista previa', async () => {
		const vm = await mount()
		expect(vm.healthPreview).toBe(
			'["CMD", "wget", "-q", "-O", "/dev/null", "http://127.0.0.1:3000/health"]',
		)

		vm.setHealthType('none')

		expect(vm.healthType).toBe('none')
		expect(vm.healthPreview).toBeNull()
		expect(vm.healthRequirement).toBe(m.catalog_config_health_req_none())
		await vm.health.form.handleSubmit()
		expect(mocks.updateApp).toHaveBeenCalledWith({
			ref,
			changes: { spec: { ...app.spec, healthcheck: { type: 'none' } } },
		})
	})

	it('los volúmenes se agregan y se quitan; el máximo es 10', async () => {
		const vm = await mount()

		vm.removeVolume(0)
		expect(vm.volumes.form.state.values.volumes).toEqual([])
		for (let index = 0; index < 10; index += 1) vm.addVolume()
		expect(vm.canAddVolume).toBe(false)
	})

	it('el timeout del deploy viaja solo si es lo único que cambió', async () => {
		const vm = await mount()

		vm.resources.form.setFieldValue('deployTimeoutSeconds', '600')
		await vm.resources.form.handleSubmit()

		expect(mocks.updateApp).toHaveBeenCalledWith({
			ref,
			changes: { deployTimeoutSeconds: 600 },
		})
	})

	it('kind_mismatch lo explica y vuelve a cargar la app', async () => {
		mocks.updateApp.mockRejectedValue(new KindMismatch())
		const client = createQueryClient({ retry: false })
		const invalidate = vi.spyOn(client, 'invalidateQueries')
		const vm = await mount(client)

		vm.run.form.setFieldValue('replicas', '3')
		await vm.run.form.handleSubmit()

		expect(vm.run.saver.formError).toBe(m.catalog_error_kind_mismatch())
		expect(invalidate).toHaveBeenCalledWith({ queryKey: catalogKeys.app(ref) })
	})

	it('un 422 de un campo de otra tarjeta, o de `spec`, va arriba de la tarjeta', async () => {
		const vm = await mount()

		mocks.updateApp.mockRejectedValueOnce(
			new ValidationError('debe estar entre 1 y 65535', 'port'),
		)
		vm.image.form.setFieldValue('image', 'nginx:1.28')
		await vm.image.form.handleSubmit()
		expect(vm.image.saver.formError).toBe(m.catalog_spec_port())

		mocks.updateApp.mockRejectedValueOnce(
			new ValidationError('JSON inválido: el campo "x" no existe', 'spec'),
		)
		vm.image.form.setFieldValue('image', 'nginx:1.29')
		await vm.image.form.handleSubmit()
		expect(vm.image.saver.formError).toBe(m.catalog_error_spec_shape())
	})

	it('la vista previa del healthcheck usa el puerto que se está editando', async () => {
		const vm = await mount()

		vm.run.form.setFieldValue('port', '8080')
		expect(vm.healthPreview).toContain('http://127.0.0.1:8080/health')

		// Un puerto a medio escribir no rompe la vista previa: vuelve al guardado.
		vm.run.form.setFieldValue('port', '0')
		expect(vm.healthPreview).toContain('http://127.0.0.1:3000/health')
	})

	it('con una tarjeta cambiada, salir pide confirmación; guardada, ya no', async () => {
		const confirm = vi.spyOn(window, 'confirm').mockReturnValue(false)
		const vm = await mount()
		const [guard] = mocks.guards
		if (!guard) throw new Error('el viewModel no registró beforeNavigate')

		const before = linkNavigation()
		guard(before)
		expect(confirm).not.toHaveBeenCalled()

		vm.resources.form.setFieldValue('deployTimeoutSeconds', '600')
		expect(vm.isDirty).toBe(true)
		const dirty = linkNavigation()
		guard(dirty)
		expect(confirm).toHaveBeenCalledWith(m.catalog_unsaved_confirm())
		expect(dirty.cancel).toHaveBeenCalled()

		await vm.resources.form.handleSubmit()
		expect(vm.isDirty).toBe(false)
		confirm.mockClear()
		const after = linkNavigation()
		guard(after)
		expect(confirm).not.toHaveBeenCalled()
		expect(after.cancel).not.toHaveBeenCalled()
	})
})
