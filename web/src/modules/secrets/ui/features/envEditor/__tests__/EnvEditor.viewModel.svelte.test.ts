import { QueryClient } from '@tanstack/svelte-query'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { render } from 'vitest-browser-svelte'
import { m } from '@/shared/ui/i18n/paraglide/messages'
import { toasts } from '@/shared/ui/stores'
import { UnavailableError, ValidationError } from '@/shared/domain'
import type { EnvEditorViewModel } from '..'
import {
	appOwner,
	type EnvKey,
	EnvOwnerNotFound,
	type EnvOwner,
	type EnvVar,
	InvalidEnvValue,
	stageOwner,
} from '../../../../domain'
import EditorHarness from './EditorHarness.svelte'

const mocks = vi.hoisted(() => ({
	getEnv: vi.fn(),
	patchEnv: vi.fn(),
}))

// Se mockea el CONTAINER, no la red: el test verifica lo que hace el viewModel con lo
// que le devuelven los casos de uso.
vi.mock('@/modules/secrets/container', () => ({
	secretsUseCases: { getEnv: mocks.getEnv, replaceEnv: vi.fn(), patchEnv: mocks.patchEnv },
}))

const at = new Date('2026-10-01T12:00:00Z')
const plain = (key: string, value: string): EnvVar => ({
	key: key as EnvKey,
	secret: false,
	value,
	updatedAt: at,
})
const secret = (key: string): EnvVar => ({
	key: key as EnvKey,
	secret: true,
	value: null,
	updatedAt: at,
})

const api = appOwner('auditfy', 'staging', 'api')
const APP_VARS = [plain('LOG_LEVEL', 'info'), plain('PORT', '3000'), secret('JWT_SECRET')]

async function mount(
	owner: EnvOwner = api,
	overridden?: ReadonlySet<string>,
): Promise<EnvEditorViewModel> {
	const client = new QueryClient({
		defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
	})
	let vm: EnvEditorViewModel | undefined
	await render(EditorHarness, {
		client,
		owner,
		...(overridden ? { overridden } : {}),
		onReady: (instance) => {
			vm = instance
		},
	})
	if (!vm) throw new Error('el harness no expuso el viewModel')
	const ready = vm
	await vi.waitFor(() => expect(ready.hasLoaded || ready.loadError !== null).toBe(true))
	return ready
}

function lastPatch() {
	return mocks.patchEnv.mock.calls.at(-1)?.[0]
}

describe('EnvEditorViewModel', () => {
	beforeEach(() => {
		vi.clearAllMocks()
		toasts.items = []
		mocks.getEnv.mockResolvedValue(APP_VARS)
		mocks.patchEnv.mockResolvedValue(undefined)
	})

	it('carga las variables y las filtra con el buscador', async () => {
		const vm = await mount()

		expect(mocks.getEnv).toHaveBeenCalledWith(api, expect.anything())
		expect(vm.count).toBe(3)
		vm.setSearch('log')
		expect(vm.visible.map((envVar) => envVar.key)).toEqual(['LOG_LEVEL'])
		vm.setSearch('')
		expect(vm.visible).toHaveLength(3)
	})

	it('un dueño que no existe y un backend caído son estados distintos', async () => {
		mocks.getEnv.mockRejectedValueOnce(new EnvOwnerNotFound())
		const missing = await mount()
		expect(missing.isNotFound).toBe(true)

		mocks.getEnv.mockRejectedValueOnce(new UnavailableError('caído'))
		const down = await mount()
		expect(down.isNotFound).toBe(false)
		expect(down.loadErrorMessage).toBe(m.common_unavailable())
	})

	describe('agregar', () => {
		it('valida la clave como el backend y no manda nada', async () => {
			const vm = await mount()

			await vm.submitAdd()
			expect(vm.newKeyError).toBe(m.common_required())

			vm.setNewKey('COMPOSE_PROFILES')
			await vm.submitAdd()
			expect(vm.newKeyError).toBe(m.secrets_error_key_reserved())

			vm.setNewKey('1BAD')
			vm.checkNewKey()
			expect(vm.newKeyError).toBe(m.secrets_error_key_format())

			vm.setNewKey('PORT')
			await vm.submitAdd()
			expect(vm.newKeyError).toBe(m.secrets_error_key_taken({ key: 'PORT' }))

			expect(mocks.patchEnv).not.toHaveBeenCalled()
		})

		it('valida el valor: el NUL no se manda', async () => {
			const vm = await mount()
			vm.setNewKey('NEW')
			vm.setNewValue('a\0b')

			await vm.submitAdd()

			expect(vm.newValueError).toBe(m.secrets_error_value_nul())
			expect(mocks.patchEnv).not.toHaveBeenCalled()
		})

		it('manda un PATCH secreto por defecto, limpia el formulario y avisa', async () => {
			const vm = await mount()
			vm.setNewKey('  API_KEY ')
			vm.setNewValue('k-1')

			await expect(vm.submitAdd()).resolves.toBe(true)

			expect(lastPatch()).toEqual({
				owner: api,
				current: APP_VARS,
				set: [['API_KEY', 'k-1']],
				secret: true,
			})
			expect(vm.newKey).toBe('')
			expect(vm.newValue).toBe('')
			expect(vm.savedChanges).toBe(1)
			expect(toasts.items.at(-1)).toMatchObject({
				tone: 'success',
				title: m.secrets_add_success(),
				description: m.secrets_saved_key_app({ key: 'API_KEY' }),
			})
		})

		it('sin marcar como secreta manda secret: false, y la siguiente vuelve a ser secreta', async () => {
			const vm = await mount()
			vm.setNewKey('PUBLIC_URL')
			vm.setNewValue('https://x')
			vm.setNewSecret(false)

			await vm.submitAdd()

			expect(lastPatch()).toMatchObject({ secret: false })
			expect(vm.newSecret).toBe(true)
		})

		it('un 422 del backend queda junto al formulario, con su motivo', async () => {
			mocks.patchEnv.mockRejectedValueOnce(
				new ValidationError('las variables suman más de 1 MiB (claves y valores)', 'set'),
			)
			const vm = await mount()
			vm.setNewKey('BIG')
			vm.setNewValue('x')

			await expect(vm.submitAdd()).resolves.toBe(false)

			expect(vm.newFormError).toBe('las variables suman más de 1 MiB (claves y valores)')
			expect(vm.newKey).toBe('BIG')
			expect(vm.savedChanges).toBe(0)

			mocks.patchEnv.mockRejectedValueOnce(new InvalidEnvValue('too_large', 'BIG', 'set'))
			await vm.submitAdd()
			expect(vm.newValueError).toBe(m.secrets_error_value_too_large())
		})

		it('un error que no es de validación va a un toast', async () => {
			mocks.patchEnv.mockRejectedValueOnce(new UnavailableError('caído'))
			const vm = await mount()
			vm.setNewKey('NEW')

			await vm.submitAdd()

			expect(vm.newFormError).toBeNull()
			expect(toasts.items.at(-1)).toMatchObject({
				tone: 'error',
				title: m.common_unavailable(),
			})
		})
	})

	describe('editar y reemplazar', () => {
		it('una no secreta se edita con su valor; sin cambios no manda nada', async () => {
			const vm = await mount()
			const port = APP_VARS[1] as EnvVar

			vm.openEdit(port)
			expect(vm.editValue).toBe('3000')
			expect(vm.editSecret).toBe(false)
			expect(vm.editHasChanges).toBe(false)
			await vm.submitEdit()
			expect(mocks.patchEnv).not.toHaveBeenCalled()
			expect(vm.editing).toBeNull()

			vm.openEdit(port)
			vm.setEditValue('8080')
			await vm.submitEdit()
			expect(lastPatch()).toMatchObject({ set: [['PORT', '8080']], secret: false })
			expect(vm.editing).toBeNull()
		})

		it('pasarla a secreta avisa que después no se va a poder leer', async () => {
			const vm = await mount()
			vm.openEdit(APP_VARS[1] as EnvVar)

			vm.setEditSecret(true)

			expect(vm.editMakesSecret).toBe(true)
			expect(vm.editHasChanges).toBe(true)
			await vm.submitEdit()
			expect(lastPatch()).toMatchObject({ set: [['PORT', '3000']], secret: true })
		})

		it('una secreta no se lee: se reemplaza con un valor nuevo obligatorio', async () => {
			const vm = await mount()
			vm.openEdit(APP_VARS[2] as EnvVar)

			expect(vm.editTitle).toBe(m.secrets_replace_title({ key: 'JWT_SECRET' }))
			expect(vm.editValue).toBe('')
			await vm.submitEdit()
			expect(vm.editError).toBe(m.secrets_error_new_value_required())
			expect(mocks.patchEnv).not.toHaveBeenCalled()

			vm.setEditValue('nuevo')
			await vm.submitEdit()
			expect(lastPatch()).toMatchObject({ set: [['JWT_SECRET', 'nuevo']], secret: true })
		})

		it('un 422 al editar queda en el modal, que sigue abierto', async () => {
			mocks.patchEnv.mockRejectedValueOnce(new ValidationError('motivo del backend', 'set'))
			const vm = await mount()
			vm.openEdit(APP_VARS[1] as EnvVar)
			vm.setEditValue('9000')

			await vm.submitEdit()

			expect(vm.editError).toBe('motivo del backend')
			expect(vm.editing?.key).toBe('PORT')
		})
	})

	describe('marcar como secreta y borrar', () => {
		it('marcar como secreta reescribe el mismo valor con secret: true', async () => {
			const vm = await mount()
			vm.askSecure(APP_VARS[0] as EnvVar)

			await vm.confirmSecure()

			expect(lastPatch()).toMatchObject({ set: [['LOG_LEVEL', 'info']], secret: true })
			expect(vm.confirmingSecret).toBeNull()
		})

		it('una secreta no se puede "marcar como secreta"', async () => {
			const vm = await mount()
			vm.askSecure(APP_VARS[2] as EnvVar)
			expect(vm.confirmingSecret).toBeNull()
		})

		it('borrar manda unset; si pisaba al stage, el texto lo dice', async () => {
			const vm = await mount(api, new Set(['LOG_LEVEL']))

			vm.askDelete(APP_VARS[0] as EnvVar)
			expect(vm.deleteBody).toBe(m.secrets_delete_body_override())
			vm.askDelete(APP_VARS[1] as EnvVar)
			expect(vm.deleteBody).toBe(m.secrets_delete_body_app())

			await vm.confirmDelete()
			expect(lastPatch()).toMatchObject({ unset: ['PORT'] })
			expect(lastPatch()).not.toHaveProperty('set')
			expect(vm.confirmingDelete).toBeNull()
		})

		it('en el stage, el texto de borrar habla de sus apps', async () => {
			const vm = await mount(stageOwner('auditfy', 'staging'))
			vm.askDelete(APP_VARS[0] as EnvVar)
			expect(vm.deleteBody).toBe(m.secrets_delete_body_stage())
		})

		it('las opciones de una secreta no ofrecen editar ni leer', async () => {
			const vm = await mount()
			const ids = (envVar: EnvVar) => vm.menuFor(envVar).map((entry) => entry.id)

			expect(ids(APP_VARS[2] as EnvVar)).toEqual(['replace', 'separator', 'delete'])
			expect(ids(APP_VARS[1] as EnvVar)).toEqual(['edit', 'secure', 'separator', 'delete'])
		})
	})

	it('un PATCH a la vez: mientras guarda, otra acción no sale', async () => {
		let release: () => void = () => {}
		mocks.patchEnv.mockImplementationOnce(
			() =>
				new Promise<void>((resolve) => {
					release = resolve
				}),
		)
		const vm = await mount()
		vm.setNewKey('A')
		const first = vm.submitAdd()

		await vi.waitFor(() => expect(vm.isAdding).toBe(true))
		vm.askDelete(APP_VARS[1] as EnvVar)
		await expect(vm.confirmDelete()).resolves.toBe(false)
		release()
		await expect(first).resolves.toBe(true)
		expect(mocks.patchEnv).toHaveBeenCalledTimes(1)
	})
})
