import { expect, test } from '@playwright/test'
import { login, seedMock, USERNAME, watch } from './support'

// El camino de siempre, contra el build con MSW y bajo la CSP: un proyecto, un stage y una
// app nuevos, una variable secreta (su valor no vuelve a la pantalla), un deploy seguido en
// Deploys hasta "Exitoso", otro, el rollback al primero y Proyectos con la app corriendo. El
// mismo recorrido que e2e-real/journey.real.ts, sin Docker. Los deploys del mock corren a
// 1/20 de su duración (`liveScale`, src/mocks/db/deploy.ts): medio segundo en vez de 10. Que
// Deploys siga uno en vivo lo prueban sus tests de viewModel.
const PROJECT = 'tienda-online'
const STAGE = 'qa'
const APP = 'web'
const APP_PATH = `/projects/${PROJECT}/stages/${STAGE}/apps/${APP}`
const SECRET = 'no-vuelve-a-la-pantalla'

test('de un proyecto nuevo al rollback: stage, app, variable secreta, deploys y rollback', async ({
	page,
}) => {
	const problems = await watch(page)
	await seedMock(page, { deploy: { liveScale: 0.05 } })
	const tabs = page.getByRole('navigation', { name: 'Secciones de la app' })
	await login(page)

	await test.step('proyecto, stage y app de formulario', async () => {
		// El slug se sugiere a partir del nombre.
		await page.getByRole('button', { name: 'Nuevo proyecto' }).first().click()
		const projectDialog = page.getByRole('dialog', { name: 'Nuevo proyecto' })
		await projectDialog.getByLabel('Nombre').fill('Tienda Online')
		await expect(projectDialog.getByLabel('Slug')).toHaveValue(PROJECT)
		await projectDialog.getByRole('button', { name: 'Crear' }).click()
		await expect(page.getByRole('heading', { name: 'Tienda Online', level: 2 })).toBeVisible()

		await page.getByRole('button', { name: `Opciones de ${PROJECT}` }).click()
		await page.getByRole('menuitem', { name: 'Nuevo stage' }).click()
		await page.getByRole('dialog').getByLabel('Slug').fill(STAGE)
		await page.getByRole('dialog').getByRole('button', { name: 'Crear' }).click()
		await expect(page.getByRole('heading', { name: STAGE, level: 3 })).toBeVisible()

		await page.getByRole('button', { name: `Nueva app en ${PROJECT}/${STAGE}` }).click()
		const appDialog = page.getByRole('dialog')
		await appDialog.getByLabel('Slug').fill(APP)
		await appDialog.getByLabel('Imagen').fill('nginx:1.27-alpine')
		await appDialog.getByLabel('Puerto').fill('80')
		await appDialog.getByRole('button', { name: 'Crear' }).click()
		await expect(page).toHaveURL(new RegExp(`${APP_PATH}/configuration$`))
		await expect(page.getByRole('heading', { name: 'Configuración', level: 1 })).toBeVisible()
	})

	await test.step('una variable secreta: el valor no vuelve a la pantalla', async () => {
		await tabs.getByRole('link', { name: 'Variables' }).click()
		await expect(page.getByRole('heading', { name: 'Variables', level: 1 })).toBeVisible()
		await page.getByRole('textbox', { name: 'Clave' }).fill('API_TOKEN')
		await page.getByRole('textbox', { name: 'Valor' }).fill(SECRET)
		await page.getByRole('button', { name: 'Agregar', exact: true }).click()
		const row = page
			.getByRole('list', { name: 'De la app' })
			.getByRole('listitem')
			.filter({ has: page.getByText('API_TOKEN', { exact: true }) })
		await expect(row).toContainText('valor secreto')
		await expect(page.getByText('Hay cambios sin desplegar.')).toBeVisible()
		// Ni en la fila ni en el formulario, que se vació.
		await expect(page.getByRole('textbox', { name: 'Valor' })).toHaveValue('')
		expect(await page.locator('main').innerText()).not.toContain(SECRET)
	})

	/** Deploys elige el deploy `id` y lo sigue, con su log, hasta "Exitoso". */
	const followUntilSucceeded = async (id: number): Promise<void> => {
		await expect(page).toHaveURL(new RegExp(`${APP_PATH}/deploys`))
		await expect(page.getByRole('heading', { level: 2, name: `#${id}` })).toBeVisible()
		const log = page.getByRole('region', { name: `Log del deploy #${id}` })
		await expect(log.getByRole('listitem').first()).toBeVisible()
		await expect(
			page.getByRole('button', { pressed: true }).filter({ hasText: 'Exitoso' }),
		).toContainText(`#${id}`, { timeout: 15_000 })
	}

	await test.step('primer deploy desde el Resumen, seguido en Deploys hasta Exitoso', async () => {
		await tabs.getByRole('link', { name: 'Resumen' }).click()
		await expect(page.getByRole('heading', { level: 1, name: APP })).toBeVisible()
		await expect(page.getByText('Esta app todavía no tiene un deploy')).toBeVisible()
		await page.getByRole('button', { name: 'Desplegar' }).first().click()
		const dialog = page.getByRole('dialog', { name: `Desplegar ${APP}` })
		await dialog.getByRole('button', { name: 'Desplegar' }).click()
		await followUntilSucceeded(1)
	})

	await test.step('segundo deploy y rollback al primero', async () => {
		await page.getByRole('button', { name: 'Desplegar' }).first().click()
		await page
			.getByRole('dialog', { name: `Desplegar ${APP}` })
			.getByRole('button', { name: 'Desplegar' })
			.click()
		await followUntilSucceeded(2)

		await tabs.getByRole('link', { name: 'Resumen' }).click()
		await page.getByRole('button', { name: 'Rollback' }).click()
		const dialog = page.getByRole('dialog', { name: 'Rollback a #1' })
		await dialog.getByRole('button', { name: 'Hacer rollback' }).click()
		await expect(page.getByText('Rollback en cola: deploy #3')).toBeVisible()
		await tabs.getByRole('link', { name: 'Deploys' }).click()
		await page.getByRole('button', { name: /#3/ }).click()
		await followUntilSucceeded(3)
		await expect(page.getByRole('button', { name: /#3/ })).toContainText(
			`rollback · ${USERNAME}`,
		)
	})

	await test.step('Proyectos muestra la app corriendo y al día', async () => {
		// De la app a una ruta sin params: el header de la app se desarma sin errores ni
		// pedidos a /projects/undefined (los junta `watch`).
		await page.getByRole('link', { name: 'muelle, inicio' }).click()
		await expect(page.getByRole('heading', { name: 'Proyectos', level: 1 })).toBeVisible()
		await expect(tabs).toHaveCount(0)
		const row = page.locator(`a[href="${APP_PATH}"]`)
		await row.scrollIntoViewIfNeeded()
		await expect(row).toContainText('Corriendo')
		await expect(row).toContainText('nginx:1.27-alpine')
		await expect(row).not.toContainText('Cambios pendientes')
	})

	expect(problems).toEqual([])
})
