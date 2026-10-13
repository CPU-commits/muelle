import { expect, test, type Page } from '@playwright/test'

// El header del AppShell con los snippets de <ShellHeader> (breadcrumb y tabs de una app,
// breadcrumb de las variables del stage). Al salir de esas rutas, los snippets no pueden
// volver a pintarse con los `params` de la ruta nueva (sin `p`, `s` ni `a`).
const USERNAME = 'felipe'
const PASSWORD = 'una-contraseña-larga'

function watchErrors(page: Page): string[] {
	const errors: string[] = []
	page.on('pageerror', (error) => errors.push(error.message))
	page.on('console', (message) => {
		// Un 4xx de fetch es esperado (CLAUDE.md, "Cosas que conviene saber").
		if (message.type() === 'error' && !message.text().startsWith('Failed to load resource')) {
			errors.push(message.text())
		}
	})
	return errors
}

async function login(page: Page): Promise<void> {
	await page.goto('/login')
	await page.getByLabel('Usuario').fill(USERNAME)
	await page.getByLabel('Contraseña').fill(PASSWORD)
	await page.getByRole('button', { name: 'Continuar' }).click()
	await expect(page.getByRole('heading', { name: 'Proyectos' })).toBeVisible()
}

const location = (page: Page) => page.getByRole('navigation', { name: 'Ubicación' })
const tabs = (page: Page) => page.getByRole('navigation', { name: 'Secciones de la app' })

test('de una app a Proyectos, al stage y entre tabs, sin errores', async ({ page }) => {
	const errors = watchErrors(page)
	await login(page)

	await page.goto('/projects/auditfy/stages/staging/apps/api/logs')
	await tabs(page).getByRole('link', { name: 'Deploys' }).click()
	await expect(page).toHaveURL(/\/apps\/api\/deploys$/)
	await expect(tabs(page).getByRole('link', { name: 'Deploys' })).toHaveAttribute(
		'aria-current',
		'page',
	)

	// App → Proyectos: la ruta nueva no tiene ningún param.
	await page.getByRole('link', { name: 'muelle, inicio' }).click()
	await expect(page).toHaveURL(/\/$/)
	await expect(page.getByRole('heading', { name: 'Proyectos' })).toBeVisible()
	await expect(tabs(page)).toHaveCount(0)

	// App → variables del stage: se queda sin `a`, y cambia un ShellHeader por otro.
	await page.goto('/projects/auditfy/stages/staging/apps/api')
	await location(page).getByRole('link', { name: 'staging' }).click()
	await expect(page).toHaveURL(/\/stages\/staging\/variables$/)
	await expect(page.getByRole('heading', { name: 'Variables del stage' })).toBeVisible()
	await expect(tabs(page)).toHaveCount(0)

	// Variables del stage → Proyectos.
	await page.getByRole('link', { name: 'muelle, inicio' }).click()
	await expect(page.getByRole('heading', { name: 'Proyectos' })).toBeVisible()

	// Y para atrás con el historial, que vuelve a montar todo.
	await page.goBack()
	await expect(page.getByRole('heading', { name: 'Variables del stage' })).toBeVisible()
	await page.goBack()
	await expect(tabs(page)).toBeVisible()

	expect(errors).toEqual([])
})

// Resumen, Deploys, Logs y Dominios (F1-deploy-ingress): de cada una a Proyectos, de vuelta
// con el historial y a las variables del stage, con sus streams y su polling corriendo.
// Ningún pedido puede salir con los params vacíos (`/projects/undefined/…`).
for (const { path, heading } of [
	{ path: '', heading: 'api' },
	{ path: '/deploys', heading: 'Deploys' },
	{ path: '/logs', heading: 'Logs' },
	{ path: '/dominios', heading: 'Dominios' },
]) {
	test(`de ${heading} (app) a Proyectos y al stage, sin errores`, async ({ page }) => {
		const errors = watchErrors(page)
		page.on('request', (request) => {
			if (request.url().includes('/undefined')) errors.push(`pedido: ${request.url()}`)
		})
		await login(page)

		await page.goto(`/projects/auditfy/stages/staging/apps/api${path}`)
		await expect(page.getByRole('heading', { level: 1, name: heading })).toBeVisible()

		await page.getByRole('link', { name: 'muelle, inicio' }).click()
		await expect(page.getByRole('heading', { name: 'Proyectos' })).toBeVisible()
		await expect(tabs(page)).toHaveCount(0)

		await page.goBack()
		await expect(page.getByRole('heading', { level: 1, name: heading })).toBeVisible()

		await location(page).getByRole('link', { name: 'staging' }).click()
		await expect(page.getByRole('heading', { name: 'Variables del stage' })).toBeVisible()
		await expect(tabs(page)).toHaveCount(0)

		expect(errors).toEqual([])
	})
}
