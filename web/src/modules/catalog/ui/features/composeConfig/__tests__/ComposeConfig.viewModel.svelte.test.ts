import type { BeforeNavigate } from '@sveltejs/kit'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createQueryClient } from '@/shared/data/query'
import { m } from '@/shared/ui/i18n/paraglide/messages'
import { toast } from '@/shared/ui/stores'
import { ValidationError } from '@/shared/domain'
import { catalogKeys } from '../../../../data'
import { appRefFrom, type ComposeApp, KindMismatch } from '../../../../domain'
import { fakeComposeApp } from '../../../../domain/repositories/__fakes__/inMemoryCatalogRepository'
import { mountViewModel, until } from '../../../__tests__/mountViewModel'
import { ComposeConfigViewModel } from '../ComposeConfig.viewModel.svelte.ts'

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

// `beforeNavigate` solo existe en el runtime de SvelteKit: se guarda el callback para
// llamarlo con una navegación de mentira.
vi.mock('$app/navigation', () => ({
	beforeNavigate: (callback: (navigation: BeforeNavigate) => void) => {
		mocks.guards.push(callback)
	},
	goto: vi.fn(),
}))

const YAML = 'services:\n  gateway:\n    image: ghcr.io/aumi-app/agents-gateway:staging\n'
const app = fakeComposeApp('aumi-agents', 'staging', 'agents', YAML) as ComposeApp
const ref = appRefFrom('aumi-agents', 'staging', 'agents')

const mount = (client = createQueryClient({ retry: false })) =>
	mountViewModel(() => new ComposeConfigViewModel(() => app), client)

function navigation(overrides: Partial<BeforeNavigate> = {}): BeforeNavigate {
	return {
		type: 'link',
		willUnload: false,
		cancel: vi.fn(),
		from: null,
		to: null,
		complete: Promise.resolve(),
		delta: undefined,
		...overrides,
	} as unknown as BeforeNavigate
}

describe('ComposeConfigViewModel', () => {
	beforeEach(() => {
		vi.clearAllMocks()
		mocks.guards.length = 0
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
		expect(vm.check).toEqual({
			kind: 'problems',
			problems: [{ line: 4, kind: 'syntax' }],
			onSave: true,
		})
		expect(vm.errorLines).toEqual([4])

		vm.setDraft('services:\n  web:\n    image: nginx\n')
		await until(() => vm.check.kind === 'idle')
	})

	it('un servicio repetido es un error: no se manda y se ve en el resumen', async () => {
		const vm = await mount()

		vm.setDraft('services:\n  web:\n    image: a\n  web:\n    image: b\n')
		await vm.save()

		expect(mocks.updateApp).not.toHaveBeenCalled()
		expect(vm.check).toEqual({
			kind: 'problems',
			problems: [{ line: 4, kind: 'duplicate_service', service: 'web' }],
			onSave: true,
		})
		expect(vm.services.map((service) => service.duplicate)).toEqual([false, true])
	})

	it('Validar avisa si la sintaxis está bien', async () => {
		const success = vi.spyOn(toast, 'success')
		const vm = await mount()

		await vm.validate()

		expect(success).toHaveBeenCalledWith(
			m.catalog_compose_valid(),
			m.catalog_compose_valid_body(),
		)
	})

	it('vacío no se manda; un 422 de compose_yaml se muestra traducido', async () => {
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
		expect(vm.check).toEqual({ kind: 'rejected', message: m.catalog_error_compose_too_large() })
	})

	it('kind_mismatch vuelve a cargar la app en vez de pedir que se recargue la página', async () => {
		mocks.updateApp.mockRejectedValue(new KindMismatch())
		const client = createQueryClient({ retry: false })
		const invalidate = vi.spyOn(client, 'invalidateQueries')
		const vm = await mount(client)

		vm.setDraft(`${YAML}# cambio\n`)
		await vm.save()

		expect(invalidate).toHaveBeenCalledWith({ queryKey: catalogKeys.app(ref) })
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

	it('con cambios sin guardar, salir pide confirmación; sin cambios, no', async () => {
		const confirm = vi.spyOn(window, 'confirm')
		await mount()
		const [guard] = mocks.guards
		if (!guard) throw new Error('el viewModel no registró beforeNavigate')

		const clean = navigation()
		guard(clean)
		expect(confirm).not.toHaveBeenCalled()
		expect(clean.cancel).not.toHaveBeenCalled()
	})

	it('si la persona dice que no, la navegación se cancela; al cerrar la pestaña, avisa el navegador', async () => {
		const confirm = vi.spyOn(window, 'confirm').mockReturnValue(false)
		const vm = await mount()
		const [guard] = mocks.guards
		if (!guard) throw new Error('el viewModel no registró beforeNavigate')
		vm.setDraft(`${YAML}# cambio\n`)

		const tab = navigation({ type: 'link' })
		guard(tab)
		expect(confirm).toHaveBeenCalledWith(m.catalog_unsaved_confirm())
		expect(tab.cancel).toHaveBeenCalled()

		confirm.mockReturnValue(true)
		const back = navigation({ type: 'popstate' })
		guard(back)
		expect(back.cancel).not.toHaveBeenCalled()

		// Recargar o cerrar: `cancel()` dispara el aviso propio del navegador (beforeunload).
		confirm.mockClear()
		const unload = navigation({ type: 'leave', willUnload: true })
		guard(unload)
		expect(confirm).not.toHaveBeenCalled()
		expect(unload.cancel).toHaveBeenCalled()

		// Un `goto` del propio panel (después de borrar la app) no pregunta.
		const programmatic = navigation({ type: 'goto' })
		guard(programmatic)
		expect(programmatic.cancel).not.toHaveBeenCalled()
	})
})
