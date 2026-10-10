import { describe, expect, it, vi } from 'vitest'
import { page, userEvent } from 'vitest/browser'
import { render } from 'vitest-browser-svelte'
import MenuHarness from './MenuHarness.svelte'

function itemRects(): DOMRect[] {
	return [...document.querySelectorAll('[role="menuitem"]')].map((item) =>
		item.getBoundingClientRect(),
	)
}

// Un item se ve si su centro cae sobre él mismo (nada lo tapa ni lo recorta).
function isHittable(element: Element): boolean {
	const rect = element.getBoundingClientRect()
	const hit = document.elementFromPoint(rect.left + rect.width / 2, rect.top + rect.height / 2)
	return hit !== null && (hit === element || element.contains(hit))
}

describe('Menu', () => {
	it('dentro de una ListRow, todos los items se ven y se pueden clickear', async () => {
		const onSelect = vi.fn()
		const screen = await render(MenuHarness, { onSelect })

		await screen.getByRole('button', { name: 'Opciones de FOO' }).click()
		const items = [...document.querySelectorAll('[role="menuitem"]')]

		expect(items).toHaveLength(3)
		for (const item of items) expect(isHittable(item)).toBe(true)

		await screen.getByRole('menuitem', { name: 'Eliminar' }).click()
		expect(onSelect).toHaveBeenCalledWith('eliminar')
		await expect.element(screen.getByRole('menu')).not.toBeInTheDocument()
	})

	it('dentro de una fila de tabla tampoco se recorta', async () => {
		const onSelect = vi.fn()
		const screen = await render(MenuHarness, { onSelect })

		await screen.getByRole('button', { name: 'Opciones de api.1' }).click()

		for (const item of document.querySelectorAll('[role="menuitem"]')) {
			expect(isHittable(item)).toBe(true)
		}
		await screen.getByRole('menuitem', { name: 'Editar' }).click()
		expect(onSelect).toHaveBeenCalledWith('editar')
	})

	it('si no hay lugar abajo, se abre hacia arriba y no se sale de la pantalla', async () => {
		const screen = await render(MenuHarness, { onSelect: vi.fn(), atBottom: true })
		const trigger = screen.getByRole('button', { name: 'Opciones abajo' })

		await trigger.click()
		const menu = document.querySelector('[role="menu"]') as HTMLElement
		const triggerRect = trigger.element().getBoundingClientRect()
		const rects = itemRects()

		expect(menu.getBoundingClientRect().bottom).toBeLessThanOrEqual(triggerRect.top)
		for (const rect of rects) {
			expect(rect.top).toBeGreaterThanOrEqual(0)
			expect(rect.bottom).toBeLessThanOrEqual(window.innerHeight)
		}
	})

	it('a 360 px el panel queda adentro de la pantalla aunque el botón esté a la izquierda', async () => {
		await page.viewport(360, 640)
		const screen = await render(MenuHarness, { onSelect: vi.fn(), atBottom: true })

		await screen.getByRole('button', { name: 'Opciones abajo' }).click()
		const panel = (document.querySelector('[role="menu"]') as HTMLElement).parentElement
		const rect = (panel as HTMLElement).getBoundingClientRect()

		expect(rect.left).toBeGreaterThanOrEqual(0)
		expect(rect.right).toBeLessThanOrEqual(360)
	})

	it('Escape cierra y devuelve el foco al botón; las flechas recorren los items', async () => {
		const screen = await render(MenuHarness, { onSelect: vi.fn() })
		const trigger = screen.getByRole('button', { name: 'Opciones de FOO' })

		await trigger.click()
		await expect.element(screen.getByRole('menuitem', { name: 'Editar' })).toHaveFocus()
		await userEvent.keyboard('{ArrowDown}')
		await expect.element(screen.getByRole('menuitem', { name: 'Ver' })).toHaveFocus()
		await userEvent.keyboard('{ArrowUp}{ArrowUp}')
		await expect.element(screen.getByRole('menuitem', { name: 'Eliminar' })).toHaveFocus()

		await userEvent.keyboard('{Escape}')
		await expect.element(screen.getByRole('menu')).not.toBeInTheDocument()
		await expect.element(trigger).toHaveFocus()
	})

	it('un clic afuera lo cierra', async () => {
		const screen = await render(MenuHarness, { onSelect: vi.fn() })

		await screen.getByRole('button', { name: 'Opciones de FOO' }).click()
		await expect.element(screen.getByRole('menu')).toBeVisible()
		// Un punto vacío de la página (el panel tapa lo que tiene abajo, a propósito).
		document.body.dispatchEvent(new PointerEvent('pointerdown', { bubbles: true }))

		await expect.element(screen.getByRole('menu')).not.toBeInTheDocument()
	})
})
