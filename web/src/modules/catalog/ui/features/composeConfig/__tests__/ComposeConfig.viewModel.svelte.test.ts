import { beforeEach, describe, expect, it, vi } from 'vitest'
import { m } from '@/shared/ui/i18n/paraglide/messages'
import { toast } from '@/shared/ui/stores'
import { ValidationError } from '@/shared/domain'
import { appRefFrom, type ComposeApp, KindMismatch } from '../../../../domain'
import { fakeComposeApp } from '../../../../domain/repositories/__fakes__/inMemoryCatalogRepository'
import { mountViewModel } from '../../../__tests__/mountViewModel'
import { ComposeConfigViewModel } from '../ComposeConfig.viewModel.svelte.ts'

const mocks = vi.hoisted(() => ({ updateApp: vi.fn() }))

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

const YAML = 'services:\n  gateway:\n    image: ghcr.io/aumi-app/agents-gateway:staging\n'
const app = fakeComposeApp('aumi-agents', 'staging', 'agents', YAML) as ComposeApp
const ref = appRefFrom('aumi-agents', 'staging', 'agents')

const mount = () => mountViewModel(() => new ComposeConfigViewModel(() => app))

describe('ComposeConfigViewModel', () => {
	beforeEach(() => {
		vi.clearAllMocks()
		mocks.updateApp.mockImplementation(async ({ changes }) => ({ ...app, ...changes }))
	})

	it('arranca con el YAML guardado y resume sus servicios', async () => {
		const vm = await mount()

		expect(vm.draft).toBe(YAML)
		expect(vm.isDirty).toBe(false)
		expect(vm.lines).toBe(4)
		expect(vm.services.map((service) => service.name)).toEqual(['gateway'])
	})

	it('guarda con PATCH compose_yaml', async () => {
		const success = vi.spyOn(toast, 'success')
		const vm = await mount()
		const next = `${YAML}  runner:\n    image: busybox\n`

		vm.setDraft(next)
		expect(vm.isDirty).toBe(true)
		await vm.save()

		expect(mocks.updateApp).toHaveBeenCalledWith({ ref, changes: { composeYaml: next } })
		expect(success).toHaveBeenCalledWith(
			m.catalog_compose_saved(),
			m.catalog_config_saved_body(),
		)
	})

	it('un error de sintaxis no se manda: marca la línea y se revalida al corregir', async () => {
		const vm = await mount()

		vm.setDraft('services:\n  web:\n    image: [nginx\n')
		await vm.save()

		expect(mocks.updateApp).not.toHaveBeenCalled()
		expect(vm.check).toEqual({ kind: 'syntax', lines: [4], onSave: true })
		expect(vm.errorLines).toEqual([4])

		vm.setDraft('services:\n  web:\n    image: nginx\n')
		expect(vm.check).toEqual({ kind: 'idle' })
	})

	it('Validar avisa si la sintaxis está bien', async () => {
		const success = vi.spyOn(toast, 'success')
		const vm = await mount()

		vm.validate()

		expect(success).toHaveBeenCalledWith(
			m.catalog_compose_valid(),
			m.catalog_compose_valid_body(),
		)
	})

	it('vacío no se manda; un 422 de compose_yaml muestra el motivo del backend', async () => {
		const vm = await mount()

		vm.setDraft('   ')
		await vm.save()
		expect(vm.check).toEqual({ kind: 'rejected', message: m.catalog_error_compose_empty() })
		expect(mocks.updateApp).not.toHaveBeenCalled()

		mocks.updateApp.mockRejectedValue(
			new ValidationError('no puede superar 256 KiB', 'composeYaml'),
		)
		vm.setDraft(`${YAML}# cambio\n`)
		await vm.save()
		expect(vm.check).toEqual({ kind: 'rejected', message: 'no puede superar 256 KiB' })
	})

	it('kind_mismatch se explica', async () => {
		mocks.updateApp.mockRejectedValue(new KindMismatch())
		const vm = await mount()

		vm.setDraft(`${YAML}# cambio\n`)
		await vm.save()

		expect(vm.check).toEqual({ kind: 'rejected', message: m.catalog_error_kind_mismatch() })
	})

	it('Descartar vuelve al YAML guardado; ir a una línea pide el foco', async () => {
		const vm = await mount()

		vm.setDraft('otra cosa')
		vm.discard()
		expect(vm.draft).toBe(YAML)
		expect(vm.isDirty).toBe(false)

		vm.goToLine(3)
		vm.goToLine(3)
		expect(vm.focusRequest).toEqual({ line: 3, nonce: 2 })
	})
})
