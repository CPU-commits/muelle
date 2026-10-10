import { describe, expect, it, vi } from 'vitest'
import { userEvent } from 'vitest/browser'
import { render } from 'vitest-browser-svelte'
import ShellHarness from './ShellHarness.svelte'

describe('AppShell', () => {
	it('muestra el logo y la nav por defecto', async () => {
		const screen = await render(ShellHarness, { override: false, onLogout: vi.fn() })

		await expect.element(screen.getByRole('link', { name: 'muelle' })).toBeInTheDocument()
		await expect.element(screen.getByRole('navigation', { name: 'Secciones' })).toBeVisible()
		await expect.element(screen.getByText('contenido', { exact: true })).toBeVisible()
	})

	it('una ruta con <ShellHeader> cambia el breadcrumb y la nav mientras está montada', async () => {
		const screen = await render(ShellHarness, { override: true, onLogout: vi.fn() })

		await expect.element(screen.getByRole('navigation', { name: 'Ubicación' })).toBeVisible()
		await expect
			.element(screen.getByRole('navigation', { name: 'Secciones de la app' }))
			.toBeVisible()
		await expect.element(screen.getByRole('link', { name: 'muelle' })).not.toBeInTheDocument()

		await screen.rerender({ override: false })

		await expect.element(screen.getByRole('link', { name: 'muelle' })).toBeInTheDocument()
		await expect
			.element(screen.getByRole('navigation', { name: 'Secciones de la app' }))
			.not.toBeInTheDocument()
	})

	it('el menú de la cuenta se abre, se cierra con Escape y ejecuta sus acciones', async () => {
		const onLogout = vi.fn()
		const screen = await render(ShellHarness, { override: false, onLogout })
		const account = screen.getByRole('button', { name: 'Cuenta: felipe' })

		await account.click()
		await expect.element(screen.getByRole('menu')).toBeVisible()
		await expect.element(screen.getByText('2FA activa')).toBeVisible()
		await expect
			.element(screen.getByRole('menuitem', { name: 'Verificación en dos pasos' }))
			.toHaveFocus()

		await userEvent.keyboard('{Escape}')
		await expect.element(screen.getByRole('menu')).not.toBeInTheDocument()
		await expect.element(account).toHaveFocus()

		await account.click()
		await screen.getByRole('menuitem', { name: 'Cerrar sesión' }).click()
		expect(onLogout).toHaveBeenCalledOnce()
		await expect.element(screen.getByRole('menu')).not.toBeInTheDocument()
	})
})
