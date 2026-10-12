import { QueryClient } from '@tanstack/svelte-query'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { render } from 'vitest-browser-svelte'
import { m } from '@/shared/ui/i18n/paraglide/messages'
import { toasts } from '@/shared/ui/stores'
import { ValidationError } from '@/shared/domain'
import type { EnvImportViewModel } from '..'
import type { EnvEditorViewModel } from '../../envEditor'
import { appOwner, type EnvKey, type EnvVar, MAX_ENV_VARS } from '../../../../domain'
import ImportHarness from './ImportHarness.svelte'

const mocks = vi.hoisted(() => ({
	getEnv: vi.fn(),
	patchEnv: vi.fn(),
}))

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

async function mount(): Promise<{ editor: EnvEditorViewModel; importer: EnvImportViewModel }> {
	const client = new QueryClient({
		defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
	})
	let vms: { editor: EnvEditorViewModel; importer: EnvImportViewModel } | undefined
	await render(ImportHarness, {
		client,
		owner: api,
		onReady: (instances) => {
			vms = instances
		},
	})
	if (!vms) throw new Error('el harness no expuso los viewModels')
	const ready = vms
	await vi.waitFor(() => expect(ready.editor.hasLoaded).toBe(true))
	ready.importer.toggle()
	return ready
}

describe('EnvImportViewModel', () => {
	beforeEach(() => {
		vi.clearAllMocks()
		toasts.items = []
		mocks.getEnv.mockResolvedValue(APP_VARS)
		mocks.patchEnv.mockResolvedValue(undefined)
	})

	it('los problemas de cada línea bloquean la importación', async () => {
		const { importer } = await mount()

		importer.setText('OK=1\nCOMPOSE_FILE=x\nesto no')

		expect(importer.issueMessages).toEqual([
			m.secrets_import_line({
				line: 2,
				message: m.secrets_dotenv_reserved_key({ key: 'COMPOSE_FILE' }),
			}),
			m.secrets_import_line({ line: 3, message: m.secrets_dotenv_missing_equals() }),
		])
		expect(importer.showPlan).toBe(false)
		expect(importer.canSubmit).toBe(false)
		await expect(importer.submit()).resolves.toBe(false)
		expect(mocks.patchEnv).not.toHaveBeenCalled()
	})

	it('combinar: el plan separa nuevas, cambios, secretas que se reescriben y sin cambios', async () => {
		const { importer } = await mount()

		importer.setText('export PORT=3000\nLOG_LEVEL=debug\nJWT_SECRET="otro"\nNEW=x\n# fin')

		expect(importer.entryCount).toBe(4)
		// Marcar como secretas está prendido por defecto: PORT y LOG_LEVEL pasan a secretas.
		expect(importer.plan.added).toEqual(['NEW'])
		expect(importer.plan.changed).toEqual([
			{ key: 'LOG_LEVEL', reason: 'to_secret' },
			{ key: 'PORT', reason: 'to_secret' },
		])
		expect(importer.plan.overwritten).toEqual(['JWT_SECRET'])

		importer.setSecret(false)
		expect(importer.plan.changed).toEqual([
			{ key: 'JWT_SECRET', reason: 'to_plain' },
			{ key: 'LOG_LEVEL', reason: 'value' },
		])
		expect(importer.plan.unchanged).toEqual(['PORT'])
	})

	it('importa en un solo PATCH, avisa y borra el texto (puede tener secretos)', async () => {
		const { importer, editor } = await mount()
		importer.setText('LOG_LEVEL=debug\nNEW="a b"')

		await expect(importer.submit()).resolves.toBe(true)

		expect(mocks.patchEnv).toHaveBeenCalledTimes(1)
		expect(mocks.patchEnv.mock.calls[0]?.[0]).toEqual({
			owner: api,
			current: APP_VARS,
			set: new Map([
				['LOG_LEVEL', 'debug'],
				['NEW', 'a b'],
			]),
			unset: [],
			secret: true,
		})
		expect(importer.open).toBe(false)
		expect(importer.text).toBe('')
		expect(editor.savedChanges).toBe(1)
		expect(toasts.items.at(-1)).toMatchObject({
			title: m.secrets_import_success(),
			description: m.secrets_imported_app(),
		})
	})

	it('reemplazar todas pide confirmar antes de borrar las que no están', async () => {
		const { importer } = await mount()
		importer.setText('PORT=3000')
		importer.setSecret(false)
		importer.setMode('replace')

		expect(importer.plan.removed).toEqual(['JWT_SECRET', 'LOG_LEVEL'])
		await expect(importer.submit()).resolves.toBe(false)
		expect(importer.confirmingReplace).toBe(true)
		expect(mocks.patchEnv).not.toHaveBeenCalled()

		await expect(importer.confirmReplace()).resolves.toBe(true)
		expect(mocks.patchEnv.mock.calls[0]?.[0]).toMatchObject({
			unset: ['JWT_SECRET', 'LOG_LEVEL'],
			set: new Map(),
		})
		expect(importer.confirmingReplace).toBe(false)
	})

	it('sin nada que cambiar no hay nada que importar', async () => {
		const { importer } = await mount()
		importer.setSecret(false)
		importer.setText('PORT=3000')

		expect(importer.showPlan).toBe(true)
		expect(importer.plan.patch).toBeNull()
		expect(importer.canSubmit).toBe(false)
	})

	it('avisa el límite de 500 variables antes de mandar', async () => {
		const { importer } = await mount()
		const lines = Array.from({ length: MAX_ENV_VARS }, (_, index) => `K${index}=v`)

		importer.setText(lines.join('\n'))

		expect(importer.limitError).toBe(m.secrets_error_limit_count({ max: MAX_ENV_VARS }))
		expect(importer.canSubmit).toBe(false)
	})

	it('un 422 queda en el panel, con el texto intacto para corregirlo', async () => {
		mocks.patchEnv.mockRejectedValueOnce(
			new ValidationError('las variables suman más de 1 MiB (claves y valores)', 'set'),
		)
		const { importer } = await mount()
		importer.setText('NEW=x')

		await expect(importer.submit()).resolves.toBe(false)

		expect(importer.serverError).toBe('las variables suman más de 1 MiB (claves y valores)')
		expect(importer.open).toBe(true)
		expect(importer.text).toBe('NEW=x')

		importer.setText('NEW=y')
		expect(importer.serverError).toBeNull()
	})

	it('cancelar cierra y olvida el texto, el modo y la marca', async () => {
		const { importer } = await mount()
		importer.setText('A=1')
		importer.setMode('replace')
		importer.setSecret(false)

		importer.toggle()

		expect(importer.open).toBe(false)
		expect(importer.text).toBe('')
		expect(importer.mode).toBe('merge')
		expect(importer.secret).toBe(true)
	})
})
