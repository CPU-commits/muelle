import { tick } from 'svelte'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { render } from 'vitest-browser-svelte'
import RouteShellHarness from './RouteShellHarness.svelte'

// Los params como los deja SvelteKit al salir del detalle de una app: a `/` sin ninguno,
// a las variables del stage sin `a`.
const APP = { p: 'auditfy', a: 'api' }
const HOME = {} as { p: string; a: string }
const STAGE = { p: 'auditfy' } as { p: string; a: string }

describe('ShellHeader al salir de su ruta', () => {
	afterEach(() => {
		vi.restoreAllMocks()
	})

	it('de una app a `/`: params vacíos y desmontaje en el mismo cambio no rompen el header', async () => {
		const consoleError = vi.spyOn(console, 'error')
		const screen = await render(RouteShellHarness, { params: APP, route: 'app' })
		await expect
			.element(screen.getByRole('navigation', { name: 'Ubicación' }))
			.toHaveTextContent('a / API')

		await screen.rerender({ params: HOME, route: 'home' })

		await expect.element(screen.getByRole('link', { name: 'muelle' })).toBeVisible()
		await expect.element(screen.getByRole('navigation', { name: 'Secciones' })).toBeVisible()
		await expect
			.element(screen.getByRole('navigation', { name: 'Ubicación' }))
			.not.toBeInTheDocument()
		await tick()
		// Es lo esperable al navegar: no es un error que haya que mostrar.
		expect(consoleError).not.toHaveBeenCalled()
	})

	it('de una app a las variables del stage: el breadcrumb del stage reemplaza al de la app', async () => {
		const consoleError = vi.spyOn(console, 'error')
		const screen = await render(RouteShellHarness, { params: APP, route: 'app' })

		await screen.rerender({ params: STAGE, route: 'stage' })

		await expect
			.element(screen.getByRole('navigation', { name: 'Ubicación' }))
			.toHaveTextContent('stage de AUDITFY')
		await expect
			.element(screen.getByRole('navigation', { name: 'Secciones de la app' }))
			.not.toBeInTheDocument()
		await expect.element(screen.getByRole('navigation', { name: 'Secciones' })).toBeVisible()
		await tick()
		expect(consoleError).not.toHaveBeenCalled()
	})

	it('vuelve a pintar los snippets cuando la ruta vuelve', async () => {
		const screen = await render(RouteShellHarness, { params: APP, route: 'app' })
		await screen.rerender({ params: HOME, route: 'home' })
		await screen.rerender({ params: { p: 'billing', a: 'web' }, route: 'app' })

		await expect
			.element(screen.getByRole('navigation', { name: 'Ubicación' }))
			.toHaveTextContent('b / WEB')
		await expect
			.element(screen.getByRole('navigation', { name: 'Secciones de la app' }))
			.toHaveTextContent('Resumen de WEB')
	})

	it('un snippet que falla con la ruta montada es un bug: va a la consola y queda el logo', async () => {
		const consoleError = vi.spyOn(console, 'error').mockImplementation(() => {})
		const screen = await render(RouteShellHarness, { params: APP, route: 'app', broken: true })

		await expect.element(screen.getByRole('link', { name: 'muelle' })).toBeVisible()
		await expect
			.poll(() =>
				consoleError.mock.calls.some(([error]) =>
					String(error).includes('bug en el breadcrumb'),
				),
			)
			.toBe(true)
		// La nav de la ruta, que no falla, sigue.
		await expect
			.element(screen.getByRole('navigation', { name: 'Secciones de la app' }))
			.toBeVisible()
	})
})
