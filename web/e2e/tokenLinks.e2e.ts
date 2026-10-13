import { expect, test, type Page } from '@playwright/test'
import { PROD_URL } from './ports'

// Los links que llegan por fuera del panel (el de la invitación, el del setup): se abren
// directo, en una pestaña nueva, con el build de producción. El token se lee y se saca de
// la URL y del historial apenas el router está listo.

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

const LINKS = [
	{
		name: 'invitación',
		link: '/invite/invitacion-demo',
		clean: '/invite',
		title: 'Unirte a muelle',
	},
	{ name: 'setup', link: '/setup?token=setup-demo', clean: '/setup', title: 'Configurar muelle' },
]

for (const { name, link, clean, title } of LINKS) {
	test(`el link de ${name} abierto directo saca el token de la URL y del historial`, async ({
		page,
	}) => {
		const errors = watchErrors(page)
		await page.goto(`${PROD_URL}/login`)
		await expect(page.getByRole('heading', { name: 'Entrar a muelle' })).toBeVisible()

		await page.goto(`${PROD_URL}${link}`)

		await expect(page.getByRole('heading', { name: title })).toBeVisible()
		await expect(page).toHaveURL(`${PROD_URL}${clean}`)
		// La entrada del link se reemplazó: atrás es el login, adelante la URL limpia.
		await page.goBack()
		await expect(page).toHaveURL(`${PROD_URL}/login`)
		await page.goForward()
		await expect(page).toHaveURL(`${PROD_URL}${clean}`)
		expect(errors).toEqual([])
	})
}
