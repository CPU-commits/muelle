import { expect, test } from '@playwright/test'
import { seedMock, watch } from './support'

// El acceso de punta a punta, contra el build con MSW y bajo la CSP: una instalación sin
// usuarios, el setup del primer usuario con el link que imprime `muelle serve`, el 2FA (el QR
// es un <svg> armado en el cliente), salir desde el menú de la cuenta (un popover en el top
// layer, posicionado por CSSOM) y entrar con la contraseña y el código. Cada test abre un
// contexto nuevo: el mock (sessionStorage) arranca con lo que siembra `seedMock`.
const USERNAME = 'admin'
const PASSWORD = 'una-contraseña-larga-e2e'
/** El link de setup del mock y el único código de 2FA que acepta (src/mocks/db/auth.ts). */
const SETUP_LINK = '/setup?token=setup-demo'
const TOTP_CODE = '123456'

test('setup del primer usuario, 2FA, salir y entrar con la contraseña y el código', async ({
	page,
}) => {
	const problems = await watch(page)
	await seedMock(page, { auth: { users: [] } })

	await test.step('sin sesión, una ruta del panel lleva al login', async () => {
		await page.goto('/settings')
		await expect(page).toHaveURL(/\/login$/)
		await expect(page.getByRole('heading', { name: 'Entrar a muelle' })).toBeVisible()
	})

	await test.step('el link de setup: el token sale de la URL y del historial', async () => {
		const before = await page.evaluate(() => history.length)
		await page.goto(SETUP_LINK)
		await expect(page.getByRole('heading', { name: 'Configurar muelle' })).toBeVisible()
		await expect(page).toHaveURL(/\/setup$/)
		// La entrada del link se reemplazó (replaceState), no se sumó otra con la URL limpia.
		expect(await page.evaluate(() => history.length)).toBe(before + 1)

		await page.getByLabel('Usuario').fill(USERNAME)
		await page.getByLabel('Contraseña', { exact: true }).fill(PASSWORD)
		await page.getByLabel('Repetir contraseña').fill(PASSWORD)
		await page.getByRole('button', { name: 'Crear usuario' }).click()
		await expect(page).toHaveURL(/\/2fa$/)
	})

	await test.step('2FA con el QR', async () => {
		await expect(page.getByRole('heading', { name: 'Configurar 2FA' })).toBeVisible()
		await expect(
			page.getByRole('img', { name: 'Código QR para la app de autenticación' }),
		).toBeVisible()
		await expect(page.locator('#totp-secret')).toHaveText(/[A-Z2-7]{4}/)
		await page.getByLabel('Código de 6 dígitos').fill(TOTP_CODE)
		await expect(page.getByText('2FA activada')).toBeVisible()
		await expect(page.getByRole('heading', { name: 'Proyectos', level: 1 })).toBeVisible()
	})

	await test.step('salir desde el menú de la cuenta', async () => {
		await page.getByRole('button', { name: `Cuenta: ${USERNAME}` }).click()
		await page.getByRole('menuitem', { name: 'Cerrar sesión' }).click()
		await expect(page).toHaveURL(/\/login$/)
	})

	await test.step('entrar con la contraseña y el código', async () => {
		await page.getByLabel('Usuario').fill(USERNAME)
		await page.getByLabel('Contraseña').fill(PASSWORD)
		await page.getByRole('button', { name: 'Continuar' }).click()
		await expect(page.getByRole('heading', { name: 'Verificación en dos pasos' })).toBeVisible()
		await page.getByLabel('Código de 6 dígitos').fill(TOTP_CODE)
		await expect(page.getByRole('heading', { name: 'Proyectos', level: 1 })).toBeVisible()
		await expect(page.getByRole('button', { name: `Cuenta: ${USERNAME}` })).toBeVisible()
	})

	expect(problems).toEqual([])
})
