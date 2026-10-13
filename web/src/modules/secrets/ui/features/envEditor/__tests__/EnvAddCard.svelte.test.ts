import { beforeEach, describe, expect, it, vi } from 'vitest'
import { userEvent } from 'vitest/browser'
import { render } from 'vitest-browser-svelte'
import { ApiError } from '@/shared/data/http/errors'
import { appOwner } from '../../../../domain'
import AddCardHarness from './AddCardHarness.svelte'

const mocks = vi.hoisted(() => ({
	get: vi.fn(),
	patch: vi.fn(),
}))

// Acá se mockea el cliente HTTP, no el container: lo que importa es el cuerpo del PATCH
// que sale del markup, pasando por el viewModel, el caso de uso y el mapper.
vi.mock('@/shared/data/http/client', async (importOriginal) => ({
	...(await importOriginal<typeof import('@/shared/data/http/client')>()),
	http: { get: mocks.get, patch: mocks.patch, put: vi.fn(), post: vi.fn(), delete: vi.fn() },
}))

const PATH = '/projects/auditfy/stages/staging/apps/api/env'
const PEM = [
	'-----BEGIN PRIVATE KEY-----',
	'MIIEvQIBADANBgkqhkiG9w0B',
	'QUJD==',
	'-----END PRIVATE KEY-----',
].join('\n')

async function mount() {
	const screen = await render(AddCardHarness, {
		owner: appOwner('auditfy', 'staging', 'api'),
	})
	await expect.element(screen.getByRole('button', { name: 'Agregar' })).toBeEnabled()
	return screen
}

describe('EnvAddCard', () => {
	beforeEach(() => {
		vi.clearAllMocks()
		mocks.get.mockResolvedValue({ vars: [] })
		mocks.patch.mockResolvedValue(null)
	})

	it('un valor pegado de varias líneas llega al PATCH con sus saltos de línea', async () => {
		const screen = await mount()

		await screen.getByRole('textbox', { name: 'Clave' }).fill('TLS_KEY')
		const value = screen.getByRole('textbox', { name: 'Valor' })
		await value.fill(PEM)
		// Un <input> de una línea se habría quedado sin los \n.
		expect((value.element() as HTMLTextAreaElement).value).toBe(PEM)
		await screen.getByRole('button', { name: 'Agregar' }).click()

		await vi.waitFor(() => expect(mocks.patch).toHaveBeenCalled())
		expect(mocks.patch).toHaveBeenCalledWith(PATH, {
			body: { set: { TLS_KEY: PEM }, secret: true },
		})
		expect(JSON.stringify(mocks.patch.mock.calls[0]?.[1])).toContain('\\n')
	})

	it('Enter agrega y Mayús + Enter es una línea nueva del valor', async () => {
		const screen = await mount()

		await screen.getByRole('textbox', { name: 'Clave' }).fill('MULTI')
		await screen.getByRole('textbox', { name: 'Valor' }).click()
		await userEvent.keyboard('uno{Shift>}{Enter}{/Shift}dos{Enter}')

		await vi.waitFor(() => expect(mocks.patch).toHaveBeenCalled())
		expect(mocks.patch).toHaveBeenCalledWith(PATH, {
			body: { set: { MULTI: 'uno\ndos' }, secret: true },
		})
	})

	it('un error del formulario marca los dos campos y los describe', async () => {
		mocks.patch.mockRejectedValueOnce(new ApiError(422, 'invalid', 'set: máximo 500 variables'))
		const screen = await mount()
		const key = screen.getByRole('textbox', { name: 'Clave' })
		const value = screen.getByRole('textbox', { name: 'Valor' })
		await key.fill('ONE_MORE')
		await screen.getByRole('button', { name: 'Agregar' }).click()

		await expect.element(screen.getByText('Máximo 500 variables.')).toBeVisible()
		for (const control of [key, value]) {
			await expect.element(control).toHaveAttribute('aria-invalid', 'true')
			await expect.element(control).toHaveAccessibleDescription(/Máximo 500 variables/)
		}
	})
})
