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

test('una lista de IPs que te deja fuera pide confirmar, y confirmada se guarda', async ({
	page,
}) => {
	const errors = watchErrors(page)
	await login(page)
	await page.goto('/ajustes')

	// El mock ve al navegador como 198.51.100.17.
	const security = page.getByRole('region', { name: 'Seguridad', exact: true })
	const cidrs = security.getByLabel('IPs permitidas para el panel')
	await cidrs.fill('10.0.0.0/8')
	await expect(security.getByText(/198\.51\.100\.17, no queda incluida/)).toBeVisible()
	await security.getByRole('button', { name: 'Guardar' }).click()

	// Lo decide el backend (409 would_lock_out): cancelar no guarda nada.
	const dialog = page.getByRole('dialog', { name: 'Te quedarías fuera del panel' })
	await expect(dialog).toBeVisible()
	await expect(dialog.getByText(/desde 198\.51\.100\.17/)).toBeVisible()
	await dialog.getByRole('button', { name: 'Cancelar' }).click()
	await expect(dialog).toHaveCount(0)
	await expect(cidrs).toHaveValue('10.0.0.0/8')

	await security.getByRole('button', { name: 'Guardar' }).click()
	await dialog.getByRole('button', { name: 'Guardar igual' }).click()
	await expect(dialog).toHaveCount(0)
	await expect(page.getByText('Seguridad guardada')).toBeVisible()
	await expect(security.getByRole('button', { name: 'Guardar' })).toBeDisabled()

	// Quedó guardado: sobrevive a recargar (el mock vive en sessionStorage).
	await page.reload()
	await expect(
		page
			.getByRole('region', { name: 'Seguridad', exact: true })
			.getByLabel('IPs permitidas para el panel'),
	).toHaveValue('10.0.0.0/8')

	expect(errors).toEqual([])
})

test('cambiar los puertos espera a que se apliquen, diciendo que está aplicando', async ({
	page,
}) => {
	const errors = watchErrors(page)
	await login(page)
	await page.goto('/ajustes')

	const ports = page.getByRole('region', { name: 'Puertos del proxy' })
	await ports.getByLabel('HTTP', { exact: true }).fill('8080')
	await ports.getByRole('button', { name: 'Guardar' }).click()

	const dialog = page.getByRole('dialog', { name: '¿Cambiar los puertos?' })
	await dialog.getByRole('button', { name: 'Cambiar puertos' }).click()
	// El mock tarda un poco en "aplicar", como el backend (que puede tardar hasta 2 min).
	await expect(dialog.getByText('Aplicando los puertos…')).toBeVisible()
	await expect(dialog).toHaveCount(0)
	await expect(page.getByText('Puertos guardados')).toBeVisible()
	await expect(ports.getByLabel('HTTP', { exact: true })).toHaveValue('8080')

	expect(errors).toEqual([])
})

test('el token de Cloudflare se puede quitar', async ({ page }) => {
	const errors = watchErrors(page)
	await login(page)
	await page.goto('/ajustes')

	const cloudflare = page.getByRole('region', { name: 'Cloudflare' })
	await expect(cloudflare.getByText('Token configurado')).toBeVisible()
	await cloudflare.getByRole('button', { name: 'Quitar token' }).click()
	const dialog = page.getByRole('dialog', { name: '¿Quitar el token de Cloudflare?' })
	await dialog.getByRole('button', { name: 'Quitar token' }).click()

	await expect(dialog).toHaveCount(0)
	await expect(cloudflare.getByText('Sin token', { exact: true })).toBeVisible()
	await expect(cloudflare.getByRole('button', { name: 'Quitar token' })).toHaveCount(0)
	await expect(cloudflare.getByRole('button', { name: 'Configurar token' })).toBeVisible()

	expect(errors).toEqual([])
})
