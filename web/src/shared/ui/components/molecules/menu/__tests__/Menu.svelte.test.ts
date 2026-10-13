import { describe, expect, it, vi } from 'vitest'
import { page, userEvent } from 'vitest/browser'
import { render } from 'vitest-browser-svelte'
import MenuConfirmHarness from './MenuConfirmHarness.svelte'
import MenuHarness from './MenuHarness.svelte'
import TallMenuHarness from './TallMenuHarness.svelte'

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

// Un menú más alto que el espacio libre (20 proyectos en un teléfono): el panel va del
// lado con más lugar, se achica a ese lugar y scrollea adentro. Todos los items se alcanzan.
describe('Menu largo a 360×640', () => {
	async function openTall(top: number) {
		await page.viewport(360, 640)
		const onSelect = vi.fn()
		const screen = await render(TallMenuHarness, { top, onSelect })
		await screen.getByRole('button', { name: 'Proyectos' }).click()
		const panel = document.querySelector('[role="menu"]')?.parentElement as HTMLElement
		return { screen, onSelect, panel }
	}

	for (const [where, top] of [
		['en el medio', 300],
		['arriba', 40],
		['abajo', 560],
	] as const) {
		it(`con el botón ${where}, el panel entra en la pantalla y scrollea`, async () => {
			const { panel } = await openTall(top)
			const rect = panel.getBoundingClientRect()

			expect(rect.top).toBeGreaterThanOrEqual(0)
			expect(rect.bottom).toBeLessThanOrEqual(640)
			expect(panel.scrollHeight).toBeGreaterThan(panel.clientHeight)
		})
	}

	it('con el teclado se llega al último item, y se ve', async () => {
		const { screen, onSelect, panel } = await openTall(300)

		await userEvent.keyboard('{End}')
		const last = screen.getByRole('menuitem', { name: 'proyecto-20' })
		await expect.element(last).toHaveFocus()
		const itemRect = last.element().getBoundingClientRect()
		const panelRect = panel.getBoundingClientRect()
		expect(itemRect.top).toBeGreaterThanOrEqual(panelRect.top)
		expect(itemRect.bottom).toBeLessThanOrEqual(panelRect.bottom)

		await userEvent.keyboard('{Enter}')
		expect(onSelect).toHaveBeenCalledWith('proyecto-20')
	})

	it('con el mouse, scrolleando el panel, se clickea el último', async () => {
		const { screen, onSelect } = await openTall(300)

		const last = screen.getByRole('menuitem', { name: 'proyecto-20' })
		last.element().scrollIntoView({ block: 'nearest' })
		expect(isHittable(last.element())).toBe(true)
		await last.click()

		expect(onSelect).toHaveBeenCalledWith('proyecto-20')
	})
})

// El item que abrió un diálogo desaparece con el menú: si el diálogo le devolviera el
// foco a él, terminaría en <body>. Antes de `onSelect` el foco ya está en el botón.
describe('Menu con un item que abre un ConfirmDialog', () => {
	it('con el mouse: Escape cierra el diálogo y el foco vuelve al botón del menú', async () => {
		const screen = await render(MenuConfirmHarness)
		const trigger = screen.getByRole('button', { name: 'Opciones de auditfy' })

		await trigger.click()
		await screen.getByRole('menuitem', { name: 'Eliminar' }).click()
		await expect.element(screen.getByRole('dialog', { name: 'Eliminar auditfy' })).toBeVisible()
		await userEvent.keyboard('{Escape}')

		await expect.element(screen.getByRole('dialog')).not.toBeInTheDocument()
		await expect.element(trigger).toHaveFocus()
	})

	it('con el teclado: Enter en el item, Cancelar, y el foco vuelve al botón del menú', async () => {
		const screen = await render(MenuConfirmHarness)
		const trigger = screen.getByRole('button', { name: 'Opciones de auditfy' })

		trigger.element().focus()
		await userEvent.keyboard('{ArrowDown}')
		await expect.element(screen.getByRole('menuitem', { name: 'Renombrar' })).toHaveFocus()
		await userEvent.keyboard('{ArrowDown}{Enter}')
		await expect.element(screen.getByRole('dialog', { name: 'Eliminar auditfy' })).toBeVisible()
		await screen.getByRole('button', { name: 'Cancelar' }).click()

		await expect.element(screen.getByRole('dialog')).not.toBeInTheDocument()
		await expect.element(trigger).toHaveFocus()
	})
})
