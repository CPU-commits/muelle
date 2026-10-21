import { expect, type Locator, type Page, test } from '@playwright/test'

// La nav de secciones de la izquierda (SubNavLayout) en App·Configuración y en Ajustes: con
// la nav al lado del contenido, acompaña el scroll (sticky) y sus links se pueden usar desde
// cualquier altura; en un teléfono, con la nav arriba del contenido, se va con la página y
// no tapa nada.
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
	await expect(page.getByRole('heading', { name: 'Proyectos', level: 1 })).toBeVisible()
}

interface Screen {
	path: string
	nav: string
	/** El id de la primera sección del contenido. */
	first: string
	/** Lo último de la página: hasta que se ve, el alto todavía puede cambiar. */
	last: (page: Page) => Locator
	/** Un link de la nav que lleva a una sección del medio, y el hash que deja. */
	jump: { name: string; hash: string }
}

const CONFIG_SCREEN: Screen = {
	path: '/projects/auditfy/stages/staging/apps/api/configuracion',
	nav: 'Secciones de configuración',
	first: 'c-imagen',
	last: (page) => page.getByRole('heading', { name: 'Eliminar app', level: 2 }),
	jump: { name: 'Volúmenes', hash: '#c-vol' },
}

const SETTINGS_SCREEN: Screen = {
	path: '/ajustes',
	nav: 'Secciones de ajustes',
	first: 'proxy',
	last: (page) => page.getByRole('heading', { name: 'Auditoría', level: 2 }),
	jump: { name: 'Seguridad', hash: '#seguridad' },
}

const SCREENS = [CONFIG_SCREEN, SETTINGS_SCREEN]

const subNav = (page: Page, name: string) => page.getByRole('navigation', { name })

async function open(page: Page, screen: Screen): Promise<void> {
	await page.goto(screen.path)
	await expect(subNav(page, screen.nav)).toBeVisible()
	await expect(screen.last(page)).toBeVisible()
}

/** Va hasta el fondo de la página (y espera a que el scroll llegue). Devuelve el scrollY. */
async function scrollToBottom(page: Page): Promise<number> {
	await page.evaluate(() => window.scrollTo(0, document.documentElement.scrollHeight))
	await expect
		.poll(() =>
			page.evaluate(
				() =>
					Math.ceil(window.scrollY + window.innerHeight) >=
					document.documentElement.scrollHeight,
			),
		)
		.toBe(true)
	return page.evaluate(() => window.scrollY)
}

/** El borde de abajo de la nav en la página (no en la pantalla), antes de scrollear. */
async function navBottomInPage(nav: Locator): Promise<number> {
	return nav.evaluate((element) => element.getBoundingClientRect().bottom + window.scrollY)
}

async function boxOf(locator: Locator): Promise<{ top: number; bottom: number }> {
	const box = await locator.boundingBox()
	if (box === null) throw new Error(`sin caja: ${locator.toString()}`)
	return { top: box.y, bottom: box.y + box.height }
}

// El centro del link cae sobre el link: nada lo tapa ni lo recorta.
async function isHittable(link: Locator): Promise<boolean> {
	return link.evaluate((element) => {
		const rect = element.getBoundingClientRect()
		const hit = document.elementFromPoint(
			rect.left + rect.width / 2,
			rect.top + rect.height / 2,
		)
		return hit !== null && (hit === element || element.contains(hit))
	})
}

async function horizontalOverflow(page: Page): Promise<number> {
	return page.evaluate(
		() => document.documentElement.scrollWidth - document.documentElement.clientWidth,
	)
}

test('con la nav al lado del contenido, acompaña el scroll y sus links se pueden usar', async ({
	page,
}) => {
	const errors = watchErrors(page)
	await page.setViewportSize({ width: 1280, height: 600 })
	await login(page)

	for (const screen of SCREENS) {
		await open(page, screen)
		const nav = subNav(page, screen.nav)
		const navBottom = await navBottomInPage(nav)

		const scrollY = await scrollToBottom(page)
		// Si la página no scrollea más que la nav, el test no probaría nada.
		expect(scrollY, `${screen.path}: la página scrollea más allá de la nav`).toBeGreaterThan(
			navBottom,
		)

		// La nav sigue entera en la pantalla, debajo del header (que no es sticky: ya se fue).
		const header = await boxOf(page.getByRole('banner'))
		const box = await boxOf(nav)
		expect(box.top, `${screen.path}: la nav está debajo del header`).toBeGreaterThanOrEqual(
			Math.max(0, header.bottom),
		)
		expect(box.bottom, `${screen.path}: la nav entra en la pantalla`).toBeLessThanOrEqual(
			await page.evaluate(() => window.innerHeight),
		)

		for (const link of await nav.getByRole('link').all()) {
			await expect(link).toBeInViewport({ ratio: 1 })
			expect(await isHittable(link), `${screen.path}: ${await link.textContent()}`).toBe(true)
		}

		// Un link lleva a su sección y la nav sigue a la vista.
		const jump = nav.getByRole('link', { name: screen.jump.name })
		await jump.click()
		await expect(page).toHaveURL(new RegExp(`${screen.jump.hash}$`))
		await expect(jump).toHaveAttribute('aria-current', 'page')
		await expect(nav).toBeInViewport({ ratio: 1 })
	}

	expect(errors).toEqual([])
})

test('una nav más alta que la pantalla scrollea sola, sin mover la página', async ({ page }) => {
	const errors = watchErrors(page)
	await page.setViewportSize({ width: 1280, height: 220 })
	await login(page)
	await open(page, CONFIG_SCREEN)
	const nav = subNav(page, CONFIG_SCREEN.nav)
	const first = nav.getByRole('link', { name: 'Imagen' })
	const last = nav.getByRole('link', { name: 'Eliminar app' })

	// A mitad de la página: la nav está pegada arriba y su último link no entra.
	await page.locator('section#c-vol').evaluate((element) => element.scrollIntoView())
	const scrollY = await page.evaluate(() => window.scrollY)
	await expect(first).toBeInViewport()
	await expect(last).not.toBeInViewport({ ratio: 1 })

	// La rueda sobre la nav la scrollea a ella: aparece el último link y la página no se mueve.
	const start = await first.boundingBox()
	if (start === null) throw new Error('el primer link no tiene caja')
	await page.mouse.move(start.x + start.width / 2, start.y + start.height / 2)
	await expect(async () => {
		await page.mouse.wheel(0, 40)
		await expect(last).toBeInViewport({ ratio: 1, timeout: 200 })
	}).toPass({ timeout: 5000 })
	expect(await page.evaluate(() => window.scrollY)).toBe(scrollY)
	expect(await isHittable(last)).toBe(true)

	expect(errors).toEqual([])
})

for (const width of [360, 390]) {
	test(`en un teléfono de ${width} px la nav se va con la página y no hay scroll horizontal`, async ({
		page,
	}) => {
		const errors = watchErrors(page)
		await page.setViewportSize({ width, height: 740 })
		await login(page)

		for (const screen of SCREENS) {
			await open(page, screen)
			const nav = subNav(page, screen.nav)
			// La nav va arriba del contenido, no al costado.
			expect((await boxOf(nav)).bottom).toBeLessThanOrEqual(
				(await boxOf(page.locator(`#${screen.first}`))).top,
			)

			await scrollToBottom(page)
			await expect(nav).not.toBeInViewport()
			expect(await horizontalOverflow(page), `${screen.path}: scroll horizontal`).toBe(0)
		}

		expect(errors).toEqual([])
	})
}
