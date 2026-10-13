import { tick } from 'svelte'
import { describe, expect, it } from 'vitest'
import { page, userEvent } from 'vitest/browser'
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

// Chrome deja cancelar un pedido de cierre (Escape → `cancel` con preventDefault) una sola
// vez sin activación del usuario: Escape no cuenta como activación, así que a partir del
// segundo cierra el <dialog> por su cuenta. Un modal de un solo uso (la URL de una
// invitación, un token nuevo) no se puede ir con su contenido.
describe('Modal dismissible={false}', () => {
	const focusInside = () => dialog()?.contains(document.activeElement) ?? false

	it('seis Escape seguidos lo dejan abierto, con el foco adentro y el contenido intacto', async () => {
		const screen = await render(ModalHarness, { dismissible: false })
		await screen.getByRole('button', { name: 'abrir' }).click()
		const input = screen.getByRole('textbox', { name: 'adentro' })
		await input.fill('https://muelle.example/invite/abc')

		for (let i = 0; i < 6; i++) await userEvent.keyboard('{Escape}')
		await tick()

		await expect.poll(() => dialog()?.open).toBe(true)
		await expect.poll(focusInside).toBe(true)
		await expect.element(input).toHaveValue('https://muelle.example/invite/abc')
		expect(text('closes')).toBe('0')
		expect(text('state')).toBe('abierto')
	})

	it('un clic en el fondo no lo cierra', async () => {
		const screen = await render(ModalHarness, { dismissible: false })
		await screen.getByRole('button', { name: 'abrir' }).click()

		await userEvent.click(screen.getByTestId('state'), { force: true })
		await tick()

		expect(dialog()?.open).toBe(true)
		expect(text('closes')).toBe('0')
	})

	it('si el navegador lo cierra igual, se vuelve a abrir con el foco donde estaba', async () => {
		const screen = await render(ModalHarness, { dismissible: false })
		await screen.getByRole('button', { name: 'abrir' }).click()
		const input = screen.getByRole('textbox', { name: 'adentro' })
		await input.click()

		dialog()?.close()

		await expect.poll(() => dialog()?.open).toBe(true)
		await expect.element(input).toHaveFocus()
		expect(text('closes')).toBe('0')
	})

	it('el dueño lo cierra con su botón', async () => {
		const screen = await render(ModalHarness, { dismissible: false })
		const opener = screen.getByRole('button', { name: 'abrir' })
		await opener.click()

		await screen.getByRole('button', { name: 'cerrar' }).click()

		await expect.element(screen.getByRole('dialog')).not.toBeInTheDocument()
		await expect.element(opener).toHaveFocus()
	})
})

describe('Modal dismissible={false}, al reabrirse solo', () => {
	it('el foco vuelve al último elemento que lo tuvo, aunque no sea el primero', async () => {
		const screen = await render(ModalHarness, { dismissible: false })
		await screen.getByRole('button', { name: 'abrir' }).click()
		await userEvent.keyboard('{Tab}')
		const cerrar = screen.getByRole('button', { name: 'cerrar' })
		await expect.element(cerrar).toHaveFocus()

		dialog()?.close()

		await expect.poll(() => dialog()?.open).toBe(true)
		await expect.element(cerrar).toHaveFocus()
	})

	it('no repite la animación de entrada', async () => {
		await render(ModalHarness, { dismissible: false }).then((screen) =>
			screen.getByRole('button', { name: 'abrir' }).click(),
		)
		await new Promise((done) => setTimeout(done, 300))

		dialog()?.close()
		await expect.poll(() => dialog()?.open).toBe(true)

		expect(dialog()?.getAnimations().length).toBe(0)
	})
})

describe('Modal en una pantalla baja (360×640)', () => {
	it('con contenido largo, el pie queda a la vista y el cuerpo scrollea', async () => {
		await page.viewport(360, 640)
		const screen = await render(ModalHarness, { tall: true })
		await screen.getByRole('button', { name: 'abrir' }).click()
		await new Promise((done) => setTimeout(done, 250))

		const cerrar = screen.getByRole('button', { name: 'cerrar' })
		const rect = cerrar.element().getBoundingClientRect()
		expect(rect.bottom).toBeLessThanOrEqual(640)
		expect(dialog()?.getBoundingClientRect().bottom).toBeLessThanOrEqual(640)
		const body = screen.getByText('Línea 1 de un formulario largo.').element().parentElement
		expect(body && body.scrollHeight > body.clientHeight).toBe(true)

		await cerrar.click()
		await expect.element(screen.getByRole('dialog')).not.toBeInTheDocument()
	})
})

describe('Modal que se cierra y se vuelve a abrir enseguida', () => {
	it('el `close` tardío del <dialog> anterior no cierra el nuevo', async () => {
		const screen = await render(ModalHarness)
		await screen.getByRole('button', { name: 'abrir' }).click()

		await screen.getByRole('button', { name: 'reabrir' }).click()
		await new Promise((done) => setTimeout(done, 50))

		expect(dialog()?.open).toBe(true)
		expect(text('state')).toBe('abierto')
		expect(text('closes')).toBe('0')
	})
})

describe('Modal dismissible (por defecto)', () => {
	it('el primer Escape lo cierra y llama a onClose una sola vez', async () => {
		const screen = await render(ModalHarness)
		await screen.getByRole('button', { name: 'abrir' }).click()

		await userEvent.keyboard('{Escape}')
		await expect.element(screen.getByRole('dialog')).not.toBeInTheDocument()
		await tick()
		await new Promise((done) => setTimeout(done, 20))

		expect(text('closes')).toBe('1')
		expect(text('state')).toBe('cerrado')
	})

	it('si el navegador lo cierra por su cuenta, avisa a onClose y el estado acompaña', async () => {
		const screen = await render(ModalHarness)
		await screen.getByRole('button', { name: 'abrir' }).click()

		dialog()?.close()

		await expect.poll(() => text('state')).toBe('cerrado')
		expect(text('closes')).toBe('1')
	})
})
