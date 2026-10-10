import { expect, test } from '@playwright/test'

// Corre contra el build real con MSW prendido (ver playwright.config.ts). El usuario
// demo es el de src/mocks/db/auth.ts. Cada test abre un contexto nuevo: la sesión del
// mock (sessionStorage) arranca vacía.
const USERNAME = 'felipe'
const PASSWORD = 'una-contraseña-larga'

test('login con el usuario demo aterriza en Proyectos', async ({ page }) => {
	await page.goto('/login')
	await expect(page.getByRole('heading', { name: 'Entrar a muelle' })).toBeVisible()

	await page.getByLabel('Usuario').fill(USERNAME)
	await page.getByLabel('Contraseña').fill(PASSWORD)
	await page.getByRole('button', { name: 'Continuar' }).click()

	await expect(page).toHaveURL(/\/$/)
	await expect(page.getByRole('heading', { name: 'Proyectos' })).toBeVisible()
	await expect(page.getByRole('button', { name: `Cuenta: ${USERNAME}` })).toBeVisible()
})

test('una contraseña mala muestra el error y no entra', async ({ page }) => {
	await page.goto('/login')

	await page.getByLabel('Usuario').fill(USERNAME)
	await page.getByLabel('Contraseña').fill('no-es-esta')
	await page.getByRole('button', { name: 'Continuar' }).click()

	await expect(page.getByRole('alert')).toHaveText('Usuario o contraseña incorrectos.')
	await expect(page).toHaveURL(/\/login$/)
})

test('una ruta protegida sin sesión redirige al login', async ({ page }) => {
	await page.goto('/ajustes')

	await expect(page).toHaveURL(/\/login$/)
	await expect(page.getByRole('heading', { name: 'Entrar a muelle' })).toBeVisible()
})

test('el detalle de una app cambia el header por el breadcrumb y sus tabs', async ({ page }) => {
	await page.goto('/login')
	await page.getByLabel('Usuario').fill(USERNAME)
	await page.getByLabel('Contraseña').fill(PASSWORD)
	await page.getByRole('button', { name: 'Continuar' }).click()
	await expect(page.getByRole('heading', { name: 'Proyectos' })).toBeVisible()

	await page.goto('/projects/auditfy/stages/staging/apps/api/logs')

	const location = page.getByRole('navigation', { name: 'Ubicación' })
	await expect(location.getByRole('link', { name: 'auditfy' })).toBeVisible()
	await expect(location.getByRole('link', { name: 'api' })).toHaveAttribute(
		'aria-current',
		'page',
	)
	const tabs = page.getByRole('navigation', { name: 'Secciones de la app' })
	await expect(tabs.getByRole('link', { name: 'Logs' })).toHaveAttribute('aria-current', 'page')

	await page.getByRole('button', { name: `Cuenta: ${USERNAME}` }).click()
	await page.getByRole('menuitem', { name: 'Cerrar sesión' }).click()
	await expect(page).toHaveURL(/\/login$/)
})
