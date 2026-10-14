import { expect, test, type Page } from '@playwright/test'

// Ajustes y Ajustes · Tokens (F1-identity-admin) sobre el build con MSW y la CSP. Sus rutas
// no tienen <ShellHeader>, pero se llega a ellas desde una que sí (el detalle de una app):
// el header tiene que volver a la nav de siempre sin errores.
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

const sections = (page: Page) => page.getByRole('navigation', { name: 'Secciones', exact: true })
const settingsNav = (page: Page) => page.getByRole('navigation', { name: 'Secciones de ajustes' })
const appTabs = (page: Page) => page.getByRole('navigation', { name: 'Secciones de la app' })

test('de una app a Ajustes y a Tokens: el header vuelve a la nav de siempre', async ({ page }) => {
	const errors = watchErrors(page)
	await login(page)

	await page.goto('/projects/auditfy/stages/staging/apps/api/logs')
	await expect(appTabs(page)).toBeVisible()
	await page.goto('/ajustes')
	await expect(page.getByRole('heading', { name: 'Ajustes', level: 1 })).toBeVisible()
	await expect(appTabs(page)).toHaveCount(0)
	await expect(
		sections(page).getByRole('link', { name: 'Ajustes', exact: true }),
	).toHaveAttribute('aria-current', 'page')

	await settingsNav(page).getByRole('link', { name: 'Usuarios' }).click()
	await expect(page).toHaveURL(/\/ajustes#usuarios$/)
	await expect(settingsNav(page).getByRole('link', { name: 'Usuarios' })).toHaveAttribute(
		'aria-current',
		'page',
	)

	await settingsNav(page).getByRole('link', { name: 'Tokens de API' }).click()
	await expect(page).toHaveURL(/\/ajustes\/tokens$/)
	await expect(page.getByRole('heading', { name: 'Tokens de API', level: 2 })).toBeVisible()

	await sections(page).getByRole('link', { name: 'Proyectos', exact: true }).click()
	await expect(page.getByRole('heading', { name: 'Proyectos' })).toBeVisible()

	expect(errors).toEqual([])
})

test('la URL de una invitación no se pierde con Escape, ni con varios seguidos', async ({
	page,
}) => {
	const errors = watchErrors(page)
	await login(page)
	await page.goto('/ajustes#usuarios')

	await page.getByRole('button', { name: 'Invitar' }).click()
	const dialog = page.getByRole('dialog', { name: 'Invitación creada' })
	const url = dialog.getByLabel('Link de invitación')
	await expect(url).toHaveValue(/\/invite\/[\w-]+$/)
	const invite = await url.inputValue()

	for (let press = 0; press < 6; press += 1) await page.keyboard.press('Escape')

	await expect(dialog).toBeVisible()
	await expect(url).toHaveValue(invite)
	await dialog.getByRole('button', { name: 'Listo' }).click()
	await expect(dialog).toHaveCount(0)

	expect(errors).toEqual([])
})
