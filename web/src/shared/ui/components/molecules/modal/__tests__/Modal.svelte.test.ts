import { tick } from 'svelte'
import { describe, expect, it } from 'vitest'
import { userEvent } from 'vitest/browser'
import { render } from 'vitest-browser-svelte'
import ModalHarness from './ModalHarness.svelte'

const dialog = () => document.querySelector('dialog') as HTMLDialogElement | null
const text = (id: string) => document.querySelector(`[data-testid="${id}"]`)?.textContent

describe('Modal', () => {
	it('Escape lo cierra y el foco vuelve al botón que lo abrió', async () => {
		const screen = await render(ModalHarness)
		const opener = screen.getByRole('button', { name: 'abrir' })

		await opener.click()
		await expect.element(screen.getByRole('dialog', { name: 'Eliminar FOO' })).toBeVisible()
		await userEvent.keyboard('{Escape}')

		await expect.element(screen.getByRole('dialog')).not.toBeInTheDocument()
		expect(text('closes')).toBe('1')
		await expect.element(opener).toHaveFocus()
	})

	it('cerrarlo desde un botón del pie también devuelve el foco', async () => {
		const screen = await render(ModalHarness)
		const opener = screen.getByRole('button', { name: 'abrir' })

		await opener.click()
		await screen.getByRole('button', { name: 'cerrar' }).click()

		await expect.element(screen.getByRole('dialog')).not.toBeInTheDocument()
		await expect.element(opener).toHaveFocus()
	})

	it('si el botón que lo abrió ya no existe, el foco va al título de la página', async () => {
		const screen = await render(ModalHarness)

		await screen.getByRole('button', { name: 'abrir' }).click()
		await screen.getByRole('button', { name: 'borrar' }).click()

		await expect.element(screen.getByRole('dialog')).not.toBeInTheDocument()
		await expect.element(screen.getByRole('heading', { name: 'Variables' })).toHaveFocus()
	})

	it('si el navegador lo cierra por su cuenta, el estado del dueño se entera', async () => {
		const screen = await render(ModalHarness)

		await screen.getByRole('button', { name: 'abrir' }).click()
		dialog()?.close()

		await expect.poll(() => text('state')).toBe('cerrado')
		await expect.element(screen.getByRole('dialog')).not.toBeInTheDocument()
	})

	it('si el dueño no deja cerrar (cargando), sigue abierto aunque se insista con Escape', async () => {
		const screen = await render(ModalHarness, { blocked: true })

		await screen.getByRole('button', { name: 'abrir' }).click()
		await userEvent.keyboard('{Escape}')
		await userEvent.keyboard('{Escape}')
		dialog()?.close()

		await expect.poll(() => dialog()?.open).toBe(true)
		expect(text('state')).toBe('abierto')
	})

	it('una selección de texto que termina sobre el fondo no lo cierra; un clic en el fondo sí', async () => {
		const screen = await render(ModalHarness)
		await screen.getByRole('button', { name: 'abrir' }).click()
		const element = dialog() as HTMLDialogElement
		const input = screen.getByRole('textbox', { name: 'adentro' }).element()

		// Se aprieta adentro (seleccionando texto) y se suelta afuera: el click llega al <dialog>.
		input.dispatchEvent(new PointerEvent('pointerdown', { bubbles: true }))
		element.dispatchEvent(new MouseEvent('click', { bubbles: true }))
		await tick()
		expect(text('closes')).toBe('0')

		element.dispatchEvent(new PointerEvent('pointerdown', { bubbles: true }))
		element.dispatchEvent(new MouseEvent('click', { bubbles: true }))
		await expect.poll(() => text('closes')).toBe('1')
	})

	// Con el mouse de verdad (Playwright): se aprieta en un campo del panel, algo cambia el
	// layout (un error de validación que desaparece) y se suelta sobre el fondo. El
	// navegador manda el click al ancestro común, el <dialog>: no tiene que cerrar.
	it('apretar adentro del panel y soltar sobre el fondo no lo cierra ni pierde lo escrito', async () => {
		const screen = await render(ModalHarness)
		await screen.getByRole('button', { name: 'abrir' }).click()
		const input = screen.getByRole('textbox', { name: 'adentro' })
		await input.fill('mi-app')

		// `state` está arriba a la izquierda, debajo del fondo del modal.
		await userEvent.dragAndDrop(input, screen.getByTestId('state'), { force: true })
		await tick()

		expect(text('closes')).toBe('0')
		expect(dialog()?.open).toBe(true)
		await expect.element(input).toHaveValue('mi-app')
	})

	it('apretar y soltar sobre el fondo, con el mouse de verdad, lo cierra', async () => {
		const screen = await render(ModalHarness)
		await screen.getByRole('button', { name: 'abrir' }).click()

		await userEvent.click(screen.getByTestId('state'), { force: true })

		await expect.poll(() => text('closes')).toBe('1')
		await expect.element(screen.getByRole('dialog')).not.toBeInTheDocument()
	})
})
