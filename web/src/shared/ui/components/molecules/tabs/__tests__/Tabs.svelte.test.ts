import { describe, expect, it } from 'vitest'
import { page } from 'vitest/browser'
import { render } from 'vitest-browser-svelte'
import type { NavItem } from '../types'
import TabsHarness from './TabsHarness.svelte'

// Los tabs de una app, con uno activo.
function tabs(active: string): NavItem[] {
	return ['Resumen', 'Deploys', 'Logs', 'Variables', 'Dominios', 'Configuración'].map(
		(label) => ({ href: `#${label}`, label, active: label === active }),
	)
}

describe('Tabs', () => {
	it('el tab activo lleva aria-current="page"; los otros, ninguno', async () => {
		const screen = await render(TabsHarness, { width: 1000, items: tabs('Logs') })
		const nav = screen.getByRole('navigation', { name: 'Secciones de la app' })

		await expect
			.element(nav.getByRole('link', { name: 'Logs' }))
			.toHaveAttribute('aria-current', 'page')
		for (const label of ['Resumen', 'Deploys', 'Variables', 'Dominios', 'Configuración']) {
			await expect
				.element(nav.getByRole('link', { name: label }))
				.not.toHaveAttribute('aria-current')
		}
	})

	it('en un teléfono la nav scrollea de costado y el tab activo queda a la vista', async () => {
		await page.viewport(360, 640)
		const screen = await render(TabsHarness, { width: 240, items: tabs('Configuración') })
		const nav = screen.getByRole('navigation', { name: 'Secciones de la app' }).element()
		const active = screen.getByRole('link', { name: 'Configuración' }).element()

		// No entran todos: el último está más allá del borde, salvo que la nav lo traiga.
		expect(nav.scrollWidth).toBeGreaterThan(nav.clientWidth)
		const box = nav.getBoundingClientRect()
		const tab = active.getBoundingClientRect()
		expect(nav.scrollLeft).toBeGreaterThan(0)
		expect(tab.left).toBeGreaterThanOrEqual(box.left)
		expect(tab.right).toBeLessThanOrEqual(box.right)
		// La página no se movió de costado por eso.
		expect(document.documentElement.scrollWidth).toBeLessThanOrEqual(
			document.documentElement.clientWidth,
		)
	})
})
