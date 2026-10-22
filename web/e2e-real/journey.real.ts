import { expect, type Page, test } from '@playwright/test'
import { realRun } from './env'
import { TotpClock } from './totp'

// El panel embebido en un `muelle serve` de verdad (-tags panel), con un Swarm de verdad, y
// solo lo que si se rompe deja a todos sin muelle: entrar, desplegar una app y volver atrás.
// Lo demás lo prueban los tests de cada módulo, no este recorrido: no se le suman pasos
// (ver "Tests" en CLAUDE.md). globalSetup.ts arranca muelle con una base nueva y deja el link
// de setup; todo lo de Docker se llama it7p<id>… y globalSetup.ts lo borra al final, aunque
// el test falle.
//
// Un solo recorrido, en orden: cada paso usa lo que dejó el anterior (el usuario, el
// proyecto, los deploys). Sin dominio del panel no hay muelle-caddy: el proxy queda
// deshabilitado y los deploys no le mandan rutas.
const PASSWORD = 'una-contraseña-larga-it7'
const STAGE = 'qa'
const APP = 'web'
const IMAGE = 'nginx:alpine'
const DEPLOY_TIMEOUT = 180_000

/** Las violaciones de CSP y los errores de la página (un 4xx de fetch es esperado). */
async function watch(page: Page): Promise<string[]> {
	const problems: string[] = []
	page.on('pageerror', (error) => problems.push(`pageerror: ${error.message}`))
	page.on('console', (message) => {
		const text = message.text()
		if (/Content Security Policy/i.test(text)) problems.push(`csp: ${text}`)
		else if (message.type() === 'error' && !text.startsWith('Failed to load resource')) {
			problems.push(`console: ${text}`)
		}
	})
	await page.exposeFunction('reportCspViolation', (text: string) => problems.push(`csp: ${text}`))
	await page.addInitScript(() => {
		document.addEventListener('securitypolicyviolation', (event) => {
			const report = (window as unknown as { reportCspViolation: (text: string) => void })
				.reportCspViolation
			report(`${event.effectiveDirective}: ${event.blockedURI}`)
		})
	})
	return problems
}

test('de la instalación al rollback: setup, 2FA, entrar, una app, su deploy corriendo y el rollback', async ({
	page,
}) => {
	test.setTimeout(10 * 60_000)
	const run = realRun()
	const { project, username } = run
	const appPath = `/projects/${project}/stages/${STAGE}/apps/${APP}`
	const totp = new TotpClock()
	const nextCode = async (secret: string): Promise<string> => {
		const wait = totp.waitMs()
		if (wait > 0) await page.waitForTimeout(wait + 500)
		return totp.next(secret).code
	}
	const problems = await watch(page)
	const tabs = page.getByRole('navigation', { name: 'Secciones de la app' })

	let secret = ''
	await test.step('setup del primer usuario con el link del log', async () => {
		await page.goto(run.setupUrl)
		await expect(page.getByRole('heading', { name: 'Configurar muelle' })).toBeVisible()
		await page.getByLabel('Usuario').fill(username)
		await page.getByLabel('Contraseña', { exact: true }).fill(PASSWORD)
		await page.getByLabel('Repetir contraseña').fill(PASSWORD)
		await page.getByRole('button', { name: 'Crear usuario' }).click()
		await expect(page).toHaveURL(/\/2fa$/)
	})

	await test.step('2FA con un código calculado de la clave que muestra el panel', async () => {
		const shown = page.locator('#totp-secret')
		await expect(shown).toHaveText(/[A-Z2-7]{4}/)
		secret = (await shown.innerText()).replace(/\s/g, '')
		await page.getByLabel('Código de 6 dígitos').fill(await nextCode(secret))
		await expect(page.getByRole('heading', { name: 'Proyectos', level: 1 })).toBeVisible()
	})

	await test.step('cerrar sesión y entrar con contraseña y código', async () => {
		await page.getByRole('button', { name: `Cuenta: ${username}` }).click()
		await page.getByRole('menuitem', { name: 'Cerrar sesión' }).click()
		await expect(page).toHaveURL(/\/login$/)
		await page.getByLabel('Usuario').fill(username)
		await page.getByLabel('Contraseña').fill(PASSWORD)
		await page.getByRole('button', { name: 'Continuar' }).click()
		await page.getByLabel('Código de 6 dígitos').fill(await nextCode(secret))
		await expect(page.getByRole('heading', { name: 'Proyectos', level: 1 })).toBeVisible()
	})

	await test.step('proyecto, stage y app de formulario', async () => {
		// Sin proyectos, el estado vacío trae otro botón igual.
		await page.getByRole('button', { name: 'Nuevo proyecto' }).first().click()
		const projectDialog = page.getByRole('dialog', { name: 'Nuevo proyecto' })
		await projectDialog.getByLabel('Nombre').fill(`it7 ${run.id}`)
		await projectDialog.getByLabel('Slug').fill(project)
		await projectDialog.getByRole('button', { name: 'Crear' }).click()
		await expect(page.getByRole('heading', { name: `it7 ${run.id}`, level: 2 })).toBeVisible()

		await page.getByRole('button', { name: `Opciones de ${project}` }).click()
		await page.getByRole('menuitem', { name: 'Nuevo stage' }).click()
		await page.getByRole('dialog').getByLabel('Slug').fill(STAGE)
		await page.getByRole('dialog').getByRole('button', { name: 'Crear' }).click()
		await expect(page.getByRole('heading', { name: STAGE, level: 3 })).toBeVisible()

		await page.getByRole('button', { name: `Nueva app en ${project}/${STAGE}` }).click()
		const appDialog = page.getByRole('dialog')
		await appDialog.getByLabel('Slug').fill(APP)
		await appDialog.getByLabel('Imagen').fill(IMAGE)
		await appDialog.getByLabel('Puerto').fill('80')
		await appDialog.getByRole('button', { name: 'Crear' }).click()
		await expect(page).toHaveURL(new RegExp(`${appPath}/configuration$`))
	})

	/** Deploys elige el deploy `id` y lo sigue hasta "Exitoso". */
	const deployAndWait = async (id: number): Promise<void> => {
		await expect(page).toHaveURL(new RegExp(`${appPath}/deploys`))
		await expect(page.getByRole('heading', { level: 2, name: `#${id}` })).toBeVisible()
		await expect(
			page.getByRole('button', { pressed: true }).filter({ hasText: 'Exitoso' }),
		).toContainText(`#${id}`, { timeout: DEPLOY_TIMEOUT })
	}

	await test.step('primer deploy, hasta Exitoso, y la app corriendo en Swarm', async () => {
		await tabs.getByRole('link', { name: 'Resumen' }).click()
		await page.getByRole('button', { name: 'Desplegar' }).first().click()
		await page
			.getByRole('dialog', { name: `Desplegar ${APP}` })
			.getByRole('button', { name: 'Desplegar' })
			.click()
		await deployAndWait(1)
		// El estado que el Resumen lee de Swarm.
		await tabs.getByRole('link', { name: 'Resumen' }).click()
		await expect(page.getByText('Corriendo').first()).toBeVisible({ timeout: 60_000 })
	})

	await test.step('segundo deploy y rollback al primero', async () => {
		await tabs.getByRole('link', { name: 'Deploys' }).click()
		await page.getByRole('button', { name: 'Desplegar' }).first().click()
		await page
			.getByRole('dialog', { name: `Desplegar ${APP}` })
			.getByRole('button', { name: 'Desplegar' })
			.click()
		await deployAndWait(2)

		await tabs.getByRole('link', { name: 'Resumen' }).click()
		await page.getByRole('button', { name: 'Rollback' }).click()
		await page
			.getByRole('dialog', { name: 'Rollback a #1' })
			.getByRole('button', { name: 'Hacer rollback' })
			.click()
		await expect(page.getByText('Rollback en cola: deploy #3')).toBeVisible()
		await tabs.getByRole('link', { name: 'Deploys' }).click()
		await page.getByRole('button', { name: /#3/ }).click()
		await deployAndWait(3)
	})

	expect(problems).toEqual([])
})
