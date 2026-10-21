import { afterEach, describe, expect, it, vi } from 'vitest'
import { userEvent } from 'vitest/browser'
import { render } from 'vitest-browser-svelte'
import { m } from '@/shared/ui/i18n/paraglide/messages'
import { toasts } from '@/shared/ui/stores'
import { aliveAfterGc } from '../../../__tests__/memory'
import CopyButtonHarness from './CopyButtonHarness.svelte'
import CopyInModalHarness from './CopyInModalHarness.svelte'

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

	// Con un Modal abierto, todo lo de afuera del <dialog> está inerte: un textarea en el
	// <body> no se puede enfocar ni seleccionar, y execCommand no copia nada.
	it('adentro de un Modal, el plan B copia desde adentro del <dialog>', async () => {
		vi.spyOn(navigator.clipboard, 'writeText').mockRejectedValue(new Error('NotAllowedError'))
		let copied: string | null = null
		vi.spyOn(document, 'execCommand').mockImplementation(() => {
			const active = document.activeElement
			const dialog = document.querySelector('dialog[open]')
			if (!(active instanceof HTMLTextAreaElement) || !dialog?.contains(active)) return false
			copied = active.value.slice(active.selectionStart, active.selectionEnd)
			return true
		})
		const screen = await render(CopyInModalHarness, { value: TOKEN })

		await screen.getByRole('button', { name: m.common_copy() }).click()

		expect(copied).toBe(TOKEN)
		await expect
			.element(screen.getByRole('button', { name: m.common_copied() }))
			.toBeInTheDocument()
		// El textarea de apoyo no queda y el foco vuelve al botón.
		expect(document.querySelectorAll('dialog textarea')).toHaveLength(0)
		await expect.element(screen.getByRole('button', { name: m.common_copied() })).toHaveFocus()
	})

	// El "Copiado" vuelve a "Copiar" con un timer (1,6 s; 3,2 s si falló). Si el modal se
	// cierra antes, ese timer no puede dejar vivo el <dialog> con el token que mostraba.
	it('cerrar el modal con el "Copiado" todavía a la vista no deja vivo el <dialog>', async () => {
		vi.spyOn(navigator.clipboard, 'writeText').mockResolvedValue(undefined)
		const screen = await render(CopyInModalHarness, { value: TOKEN })
		await screen.getByRole('button', { name: m.common_copy() }).click()
		// Con `expect.poll` y no `expect.element`: este último se queda con el elemento que
		// encontró, y el test vería esa referencia en vez de la del componente.
		await expect
			.poll(() => document.querySelector('dialog button')?.textContent?.trim())
			.toBe(m.common_copied())
		const refs = [
			new WeakRef(document.querySelector('dialog') as HTMLDialogElement),
			new WeakRef(screen.getByRole('textbox', { name: 'Token' }).element()),
		]

		await screen.rerender({ value: TOKEN, open: false })
		await expect.element(screen.getByRole('dialog')).not.toBeInTheDocument()
		// Svelte guarda el último evento delegado (el clic en "Copiar") hasta el próximo.
		await userEvent.click(screen.getByText('Tokens de API'))

		expect(await aliveAfterGc(refs)).toBe(0)
	})
})
