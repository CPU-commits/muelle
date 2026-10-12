import { describe, expect, it, vi } from 'vitest'
import { render } from 'vitest-browser-svelte'
import Breadcrumb from '../Breadcrumb.svelte'

describe('Breadcrumb', () => {
	it('los segmentos de siempre siguen siendo links o texto', async () => {
		const screen = await render(Breadcrumb, {
			label: 'Ubicación',
			homeHref: '/',
			homeLabel: 'muelle, inicio',
			items: [{ label: 'auditfy', initial: 'a', href: '/#auditfy' }, { label: 'staging' }],
		})

		await expect.element(screen.getByRole('link', { name: 'muelle, inicio' })).toBeVisible()
		await expect.element(screen.getByRole('link', { name: 'auditfy' })).toBeVisible()
		await expect.element(screen.getByText('staging')).toHaveAttribute('aria-current', 'page')
	})

	it('un segmento `menu` es el selector del diseño: botón con chevron que abre un Menu', async () => {
		const onSelect = vi.fn()
		const screen = await render(Breadcrumb, {
			label: 'Ubicación',
			homeHref: '/',
			homeLabel: 'muelle, inicio',
			items: [
				{
					kind: 'menu',
					label: 'auditfy',
					initial: 'a',
					ariaLabel: 'Proyecto auditfy, cambiar',
					items: [
						{ kind: 'link', id: 'auditfy', label: 'auditfy', href: '/#auditfy' },
						{ kind: 'action', id: 'extractor', label: 'extractor', onSelect },
					],
				},
				{ label: 'staging' },
			],
		})
		const selector = screen.getByRole('button', { name: 'Proyecto auditfy, cambiar' })

		await expect.element(selector).toHaveTextContent('auditfy')
		await expect.element(selector).toHaveAttribute('aria-haspopup', 'menu')
		await selector.click()
		await screen.getByRole('menuitem', { name: 'extractor' }).click()

		expect(onSelect).toHaveBeenCalledOnce()
	})
})
