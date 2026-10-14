import { expect, type Page, test } from '@playwright/test'

// Las variables de una app contra el build con MSW (src/mocks/handlers/secrets.ts): agregar
// (un valor de varias líneas), marcar como secreta, editar, importar un .env y borrar. Cada
// test abre un contexto nuevo: el mock (sessionStorage) arranca con los datos de ejemplo.
const USERNAME = 'felipe'
const PASSWORD = 'una-contraseña-larga'
const VARIABLES = '/projects/auditfy/stages/staging/apps/api/variables'
const PEM = [
	'-----BEGIN PRIVATE KEY-----',
	'MIIEvQIBADANBgkqhkiG9w0B',
	'QUJD==',
	'-----END PRIVATE KEY-----',
].join('\n')

async function login(page: Page): Promise<void> {
	await page.goto('/login')
	await page.getByLabel('Usuario').fill(USERNAME)
	await page.getByLabel('Contraseña').fill(PASSWORD)
	await page.getByRole('button', { name: 'Continuar' }).click()
	await expect(page).toHaveURL(/\/$/)
}

function appList(page: Page) {
	return page.getByRole('list', { name: 'De la app' })
}

function row(page: Page, key: string) {
	return appList(page)
		.getByRole('listitem')
		.filter({ has: page.getByText(key, { exact: true }) })
}

function options(page: Page, key: string) {
	return page.getByRole('button', { name: `Opciones de ${key}`, exact: true })
}

test('agregar, marcar como secreta, editar, importar y borrar variables de una app', async ({
	page,
}) => {
	await login(page)
	await page.goto(VARIABLES)
	await expect(page.getByRole('heading', { name: 'Variables', level: 1 })).toBeVisible()
	await expect(row(page, 'PORT')).toContainText('3000')

	// Agregar: el valor pegado de varias líneas viaja con sus saltos de línea.
	await page.getByRole('textbox', { name: 'Clave' }).fill('TLS_KEY')
	await page.getByRole('textbox', { name: 'Valor' }).fill(PEM)
	const added = page.waitForRequest(
		(request) => request.method() === 'PATCH' && request.url().endsWith('/apps/api/env'),
	)
	await page.getByRole('button', { name: 'Agregar', exact: true }).click()
	expect((await added).postDataJSON()).toEqual({ set: { TLS_KEY: PEM }, secret: true })
	await expect(row(page, 'TLS_KEY')).toContainText('valor secreto')
	await expect(page.getByText('Hay cambios sin desplegar.')).toBeVisible()

	// Marcar como secreta: confirma, y el foco vuelve a los ⋯ de la fila.
	await options(page, 'PORT').click()
	await page.getByRole('menuitem', { name: 'Marcar como secreta' }).click()
	await page.getByRole('dialog').getByRole('button', { name: 'Marcar como secreta' }).click()
	await expect(page.getByText('PORT ahora es secreta')).toBeVisible()
	await expect(row(page, 'PORT')).not.toContainText('3000')
	await expect(options(page, 'PORT')).toBeFocused()

	// Editar.
	await options(page, 'LOG_LEVEL').click()
	await page.getByRole('menuitem', { name: 'Editar' }).click()
	const dialog = page.getByRole('dialog')
	await dialog.getByRole('textbox', { name: 'Valor' }).fill('warn')
	await dialog.getByRole('button', { name: 'Guardar' }).click()
	await expect(row(page, 'LOG_LEVEL')).toContainText('warn')
	await expect(options(page, 'LOG_LEVEL')).toBeFocused()

	// Importar un .env, sin marcarlas como secretas.
	await page.getByRole('button', { name: 'Importar .env' }).click()
	await page
		.getByRole('textbox', { name: 'Contenido del .env' })
		.fill('# del equipo\nexport FEATURE_X=true\nNEW_ONE="a b"')
	await page.getByLabel('Marcar como secretas').uncheck()
	await expect(page.getByText('2 nuevas')).toBeVisible()
	await page.getByRole('button', { name: 'Importar', exact: true }).click()
	await expect(page.getByText('Variables importadas')).toBeVisible()
	await expect(row(page, 'FEATURE_X')).toContainText('true')
	await expect(row(page, 'NEW_ONE')).toContainText('a b')
	await expect(page.getByRole('button', { name: 'Importar .env' })).toBeFocused()

	// Borrar: la fila desaparece y el foco va al título de la lista.
	await options(page, 'NEW_ONE').click()
	await page.getByRole('menuitem', { name: 'Borrar' }).click()
	await page.getByRole('dialog').getByRole('button', { name: 'Borrar' }).click()
	await expect(row(page, 'NEW_ONE')).toHaveCount(0)
	await expect(page.getByRole('heading', { name: 'De la app' })).toBeFocused()
})

test('reemplazar una secreta sin la marca pide un segundo paso', async ({ page }) => {
	await login(page)
	await page.goto(VARIABLES)

	await options(page, 'JWT_SECRET').click()
	await page.getByRole('menuitem', { name: 'Reemplazar valor' }).click()
	const dialog = page.getByRole('dialog')
	await dialog.getByRole('textbox', { name: 'Valor nuevo' }).fill('visible')
	await dialog.getByLabel('Secreta').uncheck()
	await dialog.getByRole('button', { name: 'Guardar' }).click()

	await expect(
		dialog.getByRole('button', { name: 'Guardar sin marcar como secreta' }),
	).toBeVisible()
	await dialog.getByRole('button', { name: 'Guardar sin marcar como secreta' }).click()
	await expect(row(page, 'JWT_SECRET')).toContainText('visible')
})

test('importar con confirmación devuelve el foco a "Importar .env"', async ({ page }) => {
	await login(page)
	await page.goto(VARIABLES)

	// "Marcar como secretas" está prendido: volver secreta a PORT pide confirmar.
	await page.getByRole('button', { name: 'Importar .env' }).click()
	await page.getByRole('textbox', { name: 'Contenido del .env' }).fill('PORT=3000')
	await page.getByRole('button', { name: 'Importar', exact: true }).click()
	const dialog = page.getByRole('dialog')
	await expect(dialog.getByRole('heading', { name: 'Marcar como secretas' })).toBeVisible()
	await dialog.getByRole('button', { name: 'Importar' }).click()

	await expect(page.getByText('Variables importadas')).toBeVisible()
	await expect(row(page, 'PORT')).toContainText('valor secreto')
	await expect(page.getByRole('button', { name: 'Importar .env' })).toBeFocused()
})
