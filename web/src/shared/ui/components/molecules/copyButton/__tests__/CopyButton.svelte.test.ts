import { afterEach, describe, expect, it, vi } from 'vitest'
import { render } from 'vitest-browser-svelte'
import { m } from '@/shared/ui/i18n/paraglide/messages'
import { toasts } from '@/shared/ui/stores'
import CopyButtonHarness from './CopyButtonHarness.svelte'

const TOKEN = 'mk_Vq3tY8pL2xWc9RmZ0aKfN7sB4hJdU6gE1oTiQ5yXlCw'

describe('CopyButton', () => {
	afterEach(() => {
		vi.restoreAllMocks()
		toasts.items = []
	})

	it('copia el valor y avisa "Copiado"', async () => {
		const writeText = vi.spyOn(navigator.clipboard, 'writeText').mockResolvedValue(undefined)
		const screen = await render(CopyButtonHarness, { value: TOKEN })

		await screen.getByRole('button', { name: m.common_copy() }).click()

		expect(writeText).toHaveBeenCalledWith(TOKEN)
		await expect
			.element(screen.getByRole('button', { name: m.common_copied() }))
			.toBeInTheDocument()
	})

	it('sin portapapeles (HTTP, permiso denegado) lo dice y deja el texto seleccionado', async () => {
		vi.spyOn(navigator.clipboard, 'writeText').mockRejectedValue(new Error('NotAllowedError'))
		vi.spyOn(document, 'execCommand').mockReturnValue(false)
		const screen = await render(CopyButtonHarness, { value: TOKEN })

		await screen.getByRole('button', { name: m.common_copy() }).click()

		await expect
			.element(screen.getByRole('button', { name: m.common_copy_failed_short() }))
			.toBeInTheDocument()
		expect(toasts.items.map((item) => item.title)).toContain(m.common_copy_failed())
		const input = screen.getByRole('textbox', { name: 'Token' }).element() as HTMLInputElement
		expect(document.activeElement).toBe(input)
		expect(input.selectionStart).toBe(0)
		expect(input.selectionEnd).toBe(TOKEN.length)
	})

	it('si la API del portapapeles no está, prueba con execCommand', async () => {
		vi.spyOn(navigator.clipboard, 'writeText').mockRejectedValue(new Error('no'))
		const execCommand = vi.spyOn(document, 'execCommand').mockReturnValue(true)
		const screen = await render(CopyButtonHarness, { value: TOKEN })

		await screen.getByRole('button', { name: m.common_copy() }).click()

		expect(execCommand).toHaveBeenCalledWith('copy')
		await expect
			.element(screen.getByRole('button', { name: m.common_copied() }))
			.toBeInTheDocument()
	})
})
