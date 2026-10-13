import { QueryClient } from '@tanstack/svelte-query'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { render } from 'vitest-browser-svelte'
import { m } from '@/shared/ui/i18n/paraglide/messages'
import { toasts } from '@/shared/ui/stores'
import { UnavailableError } from '@/shared/domain'
import type { EnvEditorViewModel } from '..'
import {
	appOwner,
	EnvLimitExceeded,
	EnvOwnerNotFound,
	type EnvOwner,
	EnvRejected,
	type EnvVar,
	InvalidEnvValue,
	makeGetEnv,
	makePatchEnv,
	stageOwner,
} from '../../../../domain'
import {
	FAKE_EPOCH,
	InMemoryEnvRepository,
} from '../../../../domain/repositories/__fakes__/inMemoryEnvRepository'
import EditorHarness from './EditorHarness.svelte'

const mocks = vi.hoisted(() => ({
	getEnv: vi.fn(),
	patchEnv: vi.fn(),
}))

// Se mockea el CONTAINER, no la red: los casos de uso de verdad, sobre el fake en memoria
// del puerto (que, como el backend, no reescribe lo que no cambia).
vi.mock('@/modules/secrets/container', () => ({
	secretsUseCases: { getEnv: mocks.getEnv, replaceEnv: vi.fn(), patchEnv: mocks.patchEnv },
}))

const api = appOwner('auditfy', 'staging', 'api')
const API_LABEL = 'app:auditfy/staging/api'
const LATER = new Date('2026-10-05T12:00:00.000Z')

let fake: InMemoryEnvRepository

function seedFake(): InMemoryEnvRepository {
	return new InMemoryEnvRepository({
		owners: {
			[API_LABEL]: [
				{ key: 'JWT_SECRET', value: 'jwt-viejo', secret: true },
				{ key: 'LOG_LEVEL', value: 'info', secret: false },
				{ key: 'PORT', value: '3000', secret: false },
			],
			'stage:auditfy/staging': [{ key: 'LOG_LEVEL', value: 'debug', secret: false }],
		},
		now: () => LATER,
	})
}

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

function byKey(vm: EnvEditorViewModel, key: string): EnvVar {
	const envVar = vm.vars.find((candidate) => candidate.key === key)
	if (!envVar) throw new Error(`no está ${key}`)
	return envVar
}

describe('EnvEditorViewModel', () => {
	beforeEach(() => {
		vi.clearAllMocks()
		toasts.items = []
		fake = seedFake()
		mocks.getEnv.mockImplementation(makeGetEnv({ env: fake }))
		mocks.patchEnv.mockImplementation(makePatchEnv({ env: fake }))
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
			const before = vm.vars
			vm.setNewKey('  API_KEY ')
			vm.setNewValue('k-1')

			await expect(vm.submitAdd()).resolves.toBe(true)

			expect(lastPatch()).toEqual({
				owner: api,
				current: before,
				set: [['API_KEY', 'k-1']],
				secret: true,
			})
			expect(vm.newKey).toBe('')
			expect(vm.newValue).toBe('')
			expect(vm.savedChanges).toBe(1)
			expect(byKey(vm, 'API_KEY')).toMatchObject({ secret: true, value: null })
			expect(toasts.items.at(-1)).toMatchObject({
				tone: 'success',
				title: m.secrets_add_success(),
				description: m.secrets_saved_key_app({ key: 'API_KEY' }),
			})
		})

		it('un valor de varias líneas viaja tal cual, con sus saltos de línea', async () => {
			const pem = '-----BEGIN KEY-----\nMIIBIjAN\nQUJD==\n-----END KEY-----\n'
			const vm = await mount()
			vm.setNewKey('TLS_KEY')
			vm.setNewValue(pem)

			await vm.submitAdd()

			expect(lastPatch()).toMatchObject({ set: [['TLS_KEY', pem]] })
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

		it('un 422 queda junto al formulario, con un texto propio', async () => {
			mocks.patchEnv.mockRejectedValueOnce(new EnvLimitExceeded('size', 'set'))
			const vm = await mount()
			vm.setNewKey('BIG')
			vm.setNewValue('x')

			await expect(vm.submitAdd()).resolves.toBe(false)

			expect(vm.newFormError).toBe(m.secrets_error_limit_size())
			expect(vm.newKey).toBe('BIG')
			expect(vm.savedChanges).toBe(0)

			mocks.patchEnv.mockRejectedValueOnce(new EnvRejected('set', 'una regla nueva'))
			await vm.submitAdd()
			expect(vm.newFormError).toBe(m.secrets_error_rejected())

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

			vm.openEdit(byKey(vm, 'PORT'))
			expect(vm.editValue).toBe('3000')
			expect(vm.editSecret).toBe(false)
			expect(vm.editHasChanges).toBe(false)
			await vm.submitEdit()
			expect(mocks.patchEnv).not.toHaveBeenCalled()
			expect(vm.editing).toBeNull()

			vm.openEdit(byKey(vm, 'PORT'))
			vm.setEditValue('8080')
			await vm.submitEdit()
			expect(lastPatch()).toMatchObject({ set: [['PORT', '8080']], secret: false })
			expect(vm.editing).toBeNull()
			expect(byKey(vm, 'PORT')).toMatchObject({ value: '8080' })
		})

		it('pasarla a secreta avisa que después no se va a poder leer', async () => {
			const vm = await mount()
			vm.openEdit(byKey(vm, 'PORT'))

			vm.setEditSecret(true)

			expect(vm.editMakesSecret).toBe(true)
			expect(vm.editHasChanges).toBe(true)
			await vm.submitEdit()
			expect(lastPatch()).toMatchObject({ set: [['PORT', '3000']], secret: true })
		})

		it('una secreta no se lee: se reemplaza, también con un valor vacío', async () => {
			const vm = await mount()
			vm.openEdit(byKey(vm, 'JWT_SECRET'))

			expect(vm.editTitle).toBe(m.secrets_replace_title({ key: 'JWT_SECRET' }))
			expect(vm.editValue).toBe('')
			await vm.submitEdit()

			expect(lastPatch()).toMatchObject({ set: [['JWT_SECRET', '']], secret: true })
			expect(vm.editing).toBeNull()
		})

		it('dejarla a la vista pide un segundo paso, que se cancela al cambiar algo', async () => {
			const vm = await mount()
			vm.openEdit(byKey(vm, 'JWT_SECRET'))
			vm.setEditValue('visible')
			vm.setEditSecret(false)
			expect(vm.editRevealsSecret).toBe(true)

			await expect(vm.submitEdit()).resolves.toBe(false)
			expect(vm.editConfirmingReveal).toBe(true)
			expect(mocks.patchEnv).not.toHaveBeenCalled()

			vm.setEditValue('otro')
			expect(vm.editConfirmingReveal).toBe(false)

			await vm.submitEdit()
			await expect(vm.submitEdit()).resolves.toBe(true)
			expect(lastPatch()).toMatchObject({ set: [['JWT_SECRET', 'otro']], secret: false })
		})

		it('reescribir una secreta con el mismo valor no cuenta como cambio', async () => {
			const vm = await mount()
			vm.openEdit(byKey(vm, 'JWT_SECRET'))
			vm.setEditValue('jwt-viejo')

			await expect(vm.submitEdit()).resolves.toBe(true)

			expect(mocks.patchEnv).toHaveBeenCalledTimes(1)
			expect(vm.savedChanges).toBe(0)
			expect(toasts.items.at(-1)).toMatchObject({
				tone: 'info',
				title: m.secrets_nothing_changed(),
				description: m.secrets_nothing_changed_key({ key: 'JWT_SECRET' }),
			})
		})

		it('un 422 al editar queda en el modal, que sigue abierto', async () => {
			mocks.patchEnv.mockRejectedValueOnce(new EnvRejected('set', 'motivo del backend'))
			const vm = await mount()
			vm.openEdit(byKey(vm, 'PORT'))
			vm.setEditValue('9000')

			await vm.submitEdit()

			expect(vm.editError).toBe(m.secrets_error_rejected())
			expect(vm.editing?.key).toBe('PORT')
		})
	})

	describe('marcar como secreta y borrar', () => {
		it('marcar como secreta reescribe el mismo valor con secret: true', async () => {
			const vm = await mount()
			vm.askSecure(byKey(vm, 'LOG_LEVEL'))

			await vm.confirmSecure()

			expect(lastPatch()).toMatchObject({ set: [['LOG_LEVEL', 'info']], secret: true })
			expect(vm.confirmingSecret).toBeNull()
			expect(byKey(vm, 'LOG_LEVEL')).toMatchObject({ secret: true, value: null })
		})

		it('marcar como secreta usa el valor de la lista al confirmar, no el del diálogo', async () => {
			const vm = await mount()
			vm.askSecure(byKey(vm, 'LOG_LEVEL'))

			// Mientras el diálogo estaba abierto, otra pestaña cambió el valor.
			const stored = fake.owners.get(API_LABEL)?.find((envVar) => envVar.key === 'LOG_LEVEL')
			if (stored) stored.value = 'warn'
			vm.retry()
			await vi.waitFor(() => expect(byKey(vm, 'LOG_LEVEL')).toMatchObject({ value: 'warn' }))
			await vm.confirmSecure()

			expect(lastPatch()).toMatchObject({ set: [['LOG_LEVEL', 'warn']], secret: true })
		})

		it('una secreta no se puede "marcar como secreta"', async () => {
			const vm = await mount()
			vm.askSecure(byKey(vm, 'JWT_SECRET'))
			expect(vm.confirmingSecret).toBeNull()
		})

		it('borrar manda unset; si pisaba al stage, el texto lo dice', async () => {
			const vm = await mount(api, new Set(['LOG_LEVEL']))

			vm.askDelete(byKey(vm, 'LOG_LEVEL'))
			expect(vm.deleteBody).toBe(m.secrets_delete_body_override())
			vm.askDelete(byKey(vm, 'PORT'))
			expect(vm.deleteBody).toBe(m.secrets_delete_body_app())

			await vm.confirmDelete()
			expect(lastPatch()).toMatchObject({ unset: ['PORT'] })
			expect(lastPatch()).not.toHaveProperty('set')
			expect(vm.confirmingDelete).toBeNull()
			expect(vm.vars.map((envVar) => envVar.key)).not.toContain('PORT')
		})

		it('en el stage, el texto de borrar habla de sus apps', async () => {
			const vm = await mount(stageOwner('auditfy', 'staging'))
			vm.askDelete(byKey(vm, 'LOG_LEVEL'))
			expect(vm.deleteBody).toBe(m.secrets_delete_body_stage())
		})

		it('las opciones de una secreta no ofrecen editar ni leer', async () => {
			const vm = await mount()
			const ids = (key: string) => vm.menuFor(byKey(vm, key)).map((entry) => entry.id)

			expect(ids('JWT_SECRET')).toEqual(['replace', 'separator', 'delete'])
			expect(ids('PORT')).toEqual(['edit', 'secure', 'separator', 'delete'])
		})
	})

	describe('el foco al cerrar un diálogo de fila', () => {
		it('vuelve a los ⋯ de la fila al cancelar o guardar', async () => {
			const vm = await mount()
			expect(vm.focusSeqFor('PORT')).toBe(0)

			vm.openEdit(byKey(vm, 'PORT'))
			vm.closeEdit()
			const afterCancel = vm.focusSeqFor('PORT')
			expect(afterCancel).toBeGreaterThan(0)
			expect(vm.focusSeqFor('LOG_LEVEL')).toBe(0)

			vm.openEdit(byKey(vm, 'PORT'))
			vm.setEditValue('8080')
			await vm.submitEdit()
			expect(vm.focusSeqFor('PORT')).toBeGreaterThan(afterCancel)

			vm.askSecure(byKey(vm, 'LOG_LEVEL'))
			vm.cancelSecure()
			expect(vm.focusSeqFor('LOG_LEVEL')).toBeGreaterThan(0)

			vm.askDelete(byKey(vm, 'PORT'))
			vm.cancelDelete()
			expect(vm.focusSeqFor('PORT')).toBeGreaterThan(0)
			expect(vm.headingFocusSeq).toBe(0)
		})

		it('si la fila se borró, va al título de la lista', async () => {
			const vm = await mount()
			vm.askDelete(byKey(vm, 'PORT'))

			await vm.confirmDelete()

			expect(vm.headingFocusSeq).toBeGreaterThan(0)
			expect(vm.focusSeqFor('PORT')).toBe(0)
		})
	})

	it('un PATCH a la vez: mientras guarda, otra acción no sale', async () => {
		let release: () => void = () => {}
		const patchEnv = makePatchEnv({ env: fake })
		mocks.patchEnv.mockImplementationOnce(
			(input) =>
				new Promise<void>((resolve) => {
					release = () => void patchEnv(input).then(resolve)
				}),
		)
		const vm = await mount()
		vm.setNewKey('A')
		const first = vm.submitAdd()

		await vi.waitFor(() => expect(vm.isAdding).toBe(true))
		vm.askDelete(byKey(vm, 'PORT'))
		await expect(vm.confirmDelete()).resolves.toBe(false)
		vm.setNewKey('NEXT')
		release()
		await expect(first).resolves.toBe(true)
		expect(mocks.patchEnv).toHaveBeenCalledTimes(1)
		// Lo que se escribió mientras se guardaba no se borra.
		expect(vm.newKey).toBe('NEXT')
	})

	it('las fechas del fake son las del backend: lo que no cambió conserva la suya', async () => {
		const vm = await mount()
		vm.setNewKey('NEW')
		await vm.submitAdd()

		expect(byKey(vm, 'PORT').updatedAt).toEqual(FAKE_EPOCH)
		expect(byKey(vm, 'NEW').updatedAt).toEqual(LATER)
	})
})
