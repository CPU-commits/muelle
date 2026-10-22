import { expect, type Locator, type Page, test } from '@playwright/test'

// La nav de secciones de la izquierda (SubNavLayout) en App·Configuración y en Ajustes: con
// la nav al lado del contenido, acompaña el scroll (sticky) y sus links se pueden usar desde
// cualquier altura; en un teléfono, con la nav arriba del contenido, se va con la página y
// no tapa nada. El item marcado (aria-current) es la sección que se está leyendo: la que
// ocupa la parte de arriba de la pantalla, la última al fondo, la de un link recién seguido
// o la de un deep link.
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
	/** Las secciones (id y link), en orden, para scrollear a mano hasta cada una. */
	sections: { id: string; name: string }[]
	/**
	 * Una sección corta del final: después de saltar a ella, la página puede quedar al fondo
	 * (donde gana la última), y aun así queda marcada la del link.
	 */
	short: { id: string; name: string }
	/**
	 * Cuántas secciones, desde la primera, se pueden llevar a mano hasta la altura de la nav
	 * antes de que la página llegue al fondo (a 1280x600): las que prueba "al scrollear a
	 * mano…". Si el contenido cambia, el test lo dice en vez de probar menos sin avisar.
	 */
	reachable: number
}

const CONFIG_SCREEN: Screen = {
	path: '/projects/auditfy/stages/staging/apps/api/configuracion',
	nav: 'Secciones de configuración',
	first: 'c-imagen',
	last: (page) => page.getByRole('heading', { name: 'Eliminar app', level: 2 }),
	jump: { name: 'Volúmenes', hash: '#c-vol' },
	sections: [
		{ id: 'c-imagen', name: 'Imagen' },
		{ id: 'c-ejecucion', name: 'Ejecución' },
		{ id: 'c-health', name: 'Healthcheck' },
		{ id: 'c-vol', name: 'Volúmenes' },
		{ id: 'c-res', name: 'Recursos' },
		{ id: 'c-del', name: 'Eliminar app' },
	],
	short: { id: 'c-res', name: 'Recursos' },
	// Recursos no: la página llega al fondo antes (la prueba aparte, más abajo en la pantalla).
	reachable: 4,
}

const SETTINGS_SCREEN: Screen = {
	path: '/ajustes',
	nav: 'Secciones de ajustes',
	first: 'proxy',
	last: (page) => page.getByRole('heading', { name: 'Auditoría', level: 2 }),
	jump: { name: 'Seguridad', hash: '#seguridad' },
	sections: [
		{ id: 'proxy', name: 'Proxy y DNS' },
		{ id: 'seguridad', name: 'Seguridad' },
		{ id: 'registries', name: 'Registries' },
		{ id: 'usuarios', name: 'Usuarios' },
		{ id: 'auditoria', name: 'Auditoría' },
	],
	short: { id: 'usuarios', name: 'Usuarios' },
	reachable: 4,
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

/** El link marcado de la nav (aria-current). */
const current = (nav: Locator) => nav.locator('[aria-current="page"]')

/** Scrollea con la rueda (a mano, con el puntero sobre el contenido) hasta `y`. */
async function wheelTo(page: Page, y: number): Promise<void> {
	const { from, to, x, at } = await page.evaluate(
		(y) => ({
			from: window.scrollY,
			to: Math.max(
				0,
				Math.min(y, document.documentElement.scrollHeight - window.innerHeight),
			),
			x: window.innerWidth * 0.7,
			at: window.innerHeight / 2,
		}),
		y,
	)
	await page.mouse.move(x, at)
	await page.mouse.wheel(0, to - from)
	// Con un alto de página fraccionario, el fondo puede estar un píxel más allá.
	await expect
		.poll(() => page.evaluate((to) => Math.abs(window.scrollY - to) <= 1, to))
		.toBe(true)
}

/** Scrollea a mano hasta que el borde de arriba de la sección `id` quede a `top` px. */
async function wheelSectionTo(page: Page, id: string, top: number): Promise<void> {
	const y = await page
		.locator(`#${id}`)
		.evaluate((element, top) => window.scrollY + element.getBoundingClientRect().top - top, top)
	await wheelTo(page, y)
}

async function atTheBottom(page: Page): Promise<boolean> {
	return page.evaluate(
		() =>
			Math.ceil(window.scrollY + window.innerHeight) >= document.documentElement.scrollHeight,
	)
}

/**
 * Cuánto le falta a la sección `id` (por defecto, la del salto) para quedar a la altura del
 * primer link de la nav (pegada arriba): 0 cuando la tarjeta quedó alineada.
 */
async function alignment(
	page: Page,
	screen: Screen,
	id = screen.jump.hash.slice(1),
): Promise<number> {
	const section = await boxOf(page.locator(`#${id}`))
	const link = await boxOf(subNav(page, screen.nav).getByRole('link').first())
	return section.top - link.top
}

/** Los links de la nav pegada arriba quedan a 24 px del borde: ahí deja un salto la sección. */
const NAV_TOP = 24

/** Llegó todo lo que la pantalla carga con sus queries: ya no hay ningún "Cargando…". */
async function loaded(page: Page): Promise<void> {
	await expect(page.getByRole('status').filter({ hasText: 'Cargando…' })).toHaveCount(0)
}

/** Anota cada cambio del link marcado de la nav, para ver si pasó por otros. */
async function recordCurrent(nav: Locator): Promise<() => Promise<(string | null)[]>> {
	await nav.evaluate((element) => {
		const marks: (string | null)[] = []
		Object.assign(window, { __marks: marks })
		new MutationObserver(() => {
			const mark = element.querySelector('[aria-current="page"]')?.textContent?.trim() ?? null
			if (marks.at(-1) !== mark) marks.push(mark)
		}).observe(element, { subtree: true, attributeFilter: ['aria-current'] })
	})
	return () => nav.page().evaluate(() => (window as unknown as { __marks: string[] }).__marks)
}

/** Cuenta lo que toca el historial: pushState, replaceState y hashchange. */
async function watchHistory(page: Page): Promise<() => Promise<Record<string, number>>> {
	await page.evaluate(() => {
		const counts = { pushState: 0, replaceState: 0, hashchange: 0, length: history.length }
		Object.assign(window, { __history: counts })
		for (const method of ['pushState', 'replaceState'] as const) {
			const original = history[method].bind(history)
			history[method] = (...args: Parameters<History['pushState']>) => {
				counts[method] += 1
				return original(...args)
			}
		}
		addEventListener('hashchange', () => (counts.hashchange += 1))
	})
	return () =>
		page.evaluate(() => {
			const counts = (window as unknown as { __history: Record<string, number> }).__history
			return { ...counts, length: history.length - (counts.length ?? 0) }
		})
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

test('al scrollear a mano, la nav marca la sección que se está leyendo y, al fondo, la última', async ({
	page,
}) => {
	const errors = watchErrors(page)
	await page.setViewportSize({ width: 1280, height: 600 })
	await login(page)

	for (const screen of SCREENS) {
		await open(page, screen)
		const nav = subNav(page, screen.nav)
		const [first] = screen.sections
		await expect(current(nav), `${screen.path}: arriba de todo`).toHaveText(first!.name)

		// Cada sección, con su borde de arriba a la altura de los links de la nav (donde la deja
		// un salto), mientras la página no llegó al fondo: aunque sea corta y la siguiente ya
		// asome en la parte de arriba de la pantalla (Registries, Usuarios).
		const checked: string[] = []
		for (const section of screen.sections.slice(0, -1)) {
			await wheelSectionTo(page, section.id, NAV_TOP)
			if (await atTheBottom(page)) break
			await expect(current(nav), `${screen.path}: ${section.name}`).toHaveText(section.name)
			checked.push(section.name)
		}
		expect(checked, `${screen.path}: las que se llevaron a mano hasta la nav`).toEqual(
			screen.sections.slice(0, screen.reachable).map((section) => section.name),
		)

		// Al fondo gana la última, aunque no llegue arriba.
		await scrollToBottom(page)
		await expect(current(nav), `${screen.path}: al fondo`).toHaveText(
			screen.sections.at(-1)!.name,
		)

		await wheelTo(page, 0)
		await expect(current(nav), `${screen.path}: de vuelta arriba`).toHaveText(first!.name)
	}

	expect(errors).toEqual([])
})

test('Volúmenes y Recursos se marcan al scrollear hasta ellas, antes del fondo', async ({
	page,
}) => {
	await page.setViewportSize({ width: 1280, height: 600 })
	await login(page)
	await open(page, CONFIG_SCREEN)
	const nav = subNav(page, CONFIG_SCREEN.nav)

	await wheelSectionTo(page, 'c-vol', 150)
	await expect(current(nav)).toHaveText('Volúmenes')
	await wheelSectionTo(page, 'c-res', 150)
	// Si la página ya estuviera al fondo, ganaría "Eliminar app" y el test no probaría nada.
	expect(await atTheBottom(page)).toBe(false)
	await expect(current(nav)).toHaveText('Recursos')
})

test('con un alto de página con decimales, al fondo también gana la última', async ({ page }) => {
	const errors = watchErrors(page)
	await page.setViewportSize({ width: 1280, height: 600 })
	await login(page)
	await open(page, CONFIG_SCREEN)
	const nav = subNav(page, CONFIG_SCREEN.nav)

	// Sin healthcheck la tarjeta se achica y el alto de la página queda con decimales: al
	// fondo, el documento termina una fracción de píxel por debajo de la pantalla.
	// El radio está escondido detrás de su opción (SegmentedControl): el clic va a la opción.
	await page.getByText('Ninguno', { exact: true }).click()
	await expect(page.getByRole('radio', { name: 'Ninguno' })).toBeChecked()
	await expect
		.poll(() =>
			page.evaluate(() => document.documentElement.getBoundingClientRect().height % 1),
		)
		.not.toBe(0)

	await wheelTo(page, Number.MAX_SAFE_INTEGER)
	expect(await atTheBottom(page)).toBe(true)
	await expect(current(nav)).toHaveText('Eliminar app')

	// Un poco más arriba ya no es el fondo: vuelve la que ocupa la parte de arriba.
	await wheelTo(page, (await page.evaluate(() => window.scrollY)) - 40)
	await expect(current(nav)).toHaveText('Recursos')

	expect(errors).toEqual([])
})

test('un link de la nav marca su sección en el acto, sin pasar por otras, y scrollear no reescribe el hash', async ({
	page,
}) => {
	const errors = watchErrors(page)
	await page.setViewportSize({ width: 1280, height: 600 })
	await login(page)

	for (const screen of SCREENS) {
		await open(page, screen)
		const nav = subNav(page, screen.nav)
		const marks = await recordCurrent(nav)

		// El salto pasa por las secciones de en medio: la marca va derecho a la del link.
		const jump = nav.getByRole('link', { name: screen.jump.name })
		await jump.click()
		await expect(page).toHaveURL(new RegExp(`${screen.jump.hash}$`))
		await expect(current(nav)).toHaveText(screen.jump.name)
		// La tarjeta queda a la altura de los links de la nav pegada arriba.
		await expect
			.poll(() => alignment(page, screen), `${screen.path}: la sección alineada`)
			.toBeCloseTo(0, 0)
		await page.waitForTimeout(300)
		expect(await marks(), `${screen.path}: sin parpadeos`).toEqual([screen.jump.name])

		// Una corta del final: la página puede quedar al fondo (donde gana la última), y la
		// marca sigue en la del link hasta que se scrollea a mano.
		const short = nav.getByRole('link', { name: screen.short.name })
		await short.click()
		await expect(current(nav)).toHaveText(screen.short.name)
		await page.waitForTimeout(300)
		await expect(current(nav), `${screen.path}: ${screen.short.name} sigue`).toHaveText(
			screen.short.name,
		)
		expect(await marks(), `${screen.path}: sin parpadeos`).toEqual([
			screen.jump.name,
			screen.short.name,
		])

		// Scrollear a mano: la marca sigue al scroll y el hash no se toca (ni el historial).
		const history = await watchHistory(page)
		await wheelSectionTo(page, screen.jump.hash.slice(1), 150)
		await expect(current(nav)).toHaveText(screen.jump.name)
		await scrollToBottom(page)
		await expect(current(nav)).toHaveText(screen.sections.at(-1)!.name)
		await wheelTo(page, 0)
		await expect(current(nav)).toHaveText(screen.sections[0]!.name)
		await expect(page).toHaveURL(new RegExp(`#${screen.short.id}$`))
		expect(await history(), `${screen.path}: el historial`).toEqual({
			pushState: 0,
			replaceState: 0,
			hashchange: 0,
			length: 0,
		})
	}

	expect(errors).toEqual([])
})

test('volver atrás después de un link marca lo que se ve, no la sección del link', async ({
	page,
}) => {
	const errors = watchErrors(page)
	await page.setViewportSize({ width: 1280, height: 600 })
	await login(page)
	await open(page, SETTINGS_SCREEN)
	await loaded(page)
	const nav = subNav(page, SETTINGS_SCREEN.nav)

	// Volver atrás antes de que SvelteKit anote la entrada del historial (en el hashchange) no
	// recupera el scroll: se espera a que el link termine de navegar, como lo haría una persona.
	for (const name of ['Usuarios', 'Seguridad']) {
		const navigated = page.evaluate(
			() =>
				new Promise<void>((resolve) =>
					addEventListener('hashchange', () => setTimeout(resolve), { once: true }),
				),
		)
		await nav.getByRole('link', { name }).click()
		await navigated
		await expect(current(nav)).toHaveText(name)
	}

	// El historial vuelve a donde estaba el scroll: Usuarios (corta) arriba, a la altura de la
	// nav, aunque Auditoría asome debajo.
	await page.goBack()
	await expect(page).toHaveURL(/#usuarios$/)
	await expect.poll(() => alignment(page, SETTINGS_SCREEN, 'usuarios')).toBeCloseTo(0, 0)
	await expect(current(nav)).toHaveText('Usuarios')

	// Y arriba de todo, la primera: la marca del link de Seguridad no se queda.
	await page.goBack()
	await expect(page).toHaveURL(/\/ajustes$/)
	await expect.poll(() => page.evaluate(() => window.scrollY)).toBe(0)
	await expect(current(nav)).toHaveText('Proxy y DNS')

	expect(errors).toEqual([])
})

test('un deep link abre en su sección, marcada, aunque llegue después que la página', async ({
	page,
}) => {
	const errors = watchErrors(page)
	await page.setViewportSize({ width: 1280, height: 600 })
	await login(page)

	for (const screen of SCREENS) {
		const nav = subNav(page, screen.nav)
		await page.goto(`${screen.path}${screen.jump.hash}`)
		await expect(current(nav)).toHaveText(screen.jump.name)
		await expect
			.poll(() => alignment(page, screen), `${screen.path}: abre en la sección`)
			.toBeCloseTo(0, 0)
		expect(await page.evaluate(() => window.scrollY)).toBeGreaterThan(0)
		await expect(screen.last(page)).toBeVisible()
		await expect(current(nav)).toHaveText(screen.jump.name)
	}

	// Una que no llega arriba: la página va al fondo y la marcada es la del ancla.
	const nav = subNav(page, CONFIG_SCREEN.nav)
	await page.goto(`${CONFIG_SCREEN.path}#${CONFIG_SCREEN.short.id}`)
	await expect(current(nav)).toHaveText(CONFIG_SCREEN.short.name)
	await expect.poll(() => atTheBottom(page)).toBe(true)
	await page.waitForTimeout(300)
	await expect(current(nav)).toHaveText(CONFIG_SCREEN.short.name)

	expect(errors).toEqual([])
})

for (const viewport of [
	{ width: 1280, height: 600 },
	{ width: 1440, height: 900 },
]) {
	test(`en Ajustes, un deep link abre en su sección, marcada, aunque arriba llegue contenido después (${viewport.width}x${viewport.height})`, async ({
		page,
	}) => {
		const errors = watchErrors(page)
		await page.setViewportSize(viewport)
		await login(page)
		const nav = subNav(page, SETTINGS_SCREEN.nav)

		// Registries, Usuarios y Auditoría están en la página desde el principio; arriba, Proxy
		// y DNS y Seguridad llegan con los ajustes (más de una pantalla), y cada lista con su
		// query.
		for (const section of SETTINGS_SCREEN.sections.slice(2)) {
			await page.goto(`${SETTINGS_SCREEN.path}#${section.id}`)
			await expect(current(nav), section.id).toHaveText(section.name)
			await loaded(page)
			await expect
				.poll(() => alignment(page, SETTINGS_SCREEN, section.id), `${section.id}: alineada`)
				.toBeCloseTo(0, 0)
			await page.waitForTimeout(300)
			await expect(current(nav), section.id).toHaveText(section.name)
			expect(await alignment(page, SETTINGS_SCREEN, section.id), section.id).toBeCloseTo(0, 0)
		}

		expect(errors).toEqual([])
	})
}

test('un link de la nav desde Tokens scrolleada lleva a su sección de Ajustes, marcada', async ({
	page,
}) => {
	const errors = watchErrors(page)
	await page.setViewportSize({ width: 1280, height: 600 })
	await login(page)
	const nav = subNav(page, SETTINGS_SCREEN.nav)
	const tokensHeading = page.getByRole('heading', { name: 'Tokens de API', level: 2 })

	// La primera vez los ajustes no están en el cache (Seguridad llega después que la página);
	// la segunda, sí.
	await page.goto('/ajustes/tokens')
	for (const section of [
		{ id: 'seguridad', name: 'Seguridad' },
		{ id: 'usuarios', name: 'Usuarios' },
	]) {
		await expect(tokensHeading).toBeVisible()
		await loaded(page)
		await wheelTo(page, Number.MAX_SAFE_INTEGER)
		expect(await page.evaluate(() => window.scrollY), 'Tokens scrolleada').toBeGreaterThan(0)

		await nav.getByRole('link', { name: section.name }).click()
		await expect(page).toHaveURL(new RegExp(`/ajustes#${section.id}$`))
		await expect(current(nav), section.id).toHaveText(section.name)
		await loaded(page)
		await expect
			.poll(() => alignment(page, SETTINGS_SCREEN, section.id), `${section.id}: alineada`)
			.toBeCloseTo(0, 0)
		await page.waitForTimeout(300)
		await expect(current(nav), section.id).toHaveText(section.name)
		expect(await alignment(page, SETTINGS_SCREEN, section.id), section.id).toBeCloseTo(0, 0)

		await nav.getByRole('link', { name: 'Tokens de API' }).click()
	}

	expect(errors).toEqual([])
})

test('en un teléfono la marca también sigue al scroll, sin mover la página ni el layout', async ({
	page,
}) => {
	const errors = watchErrors(page)
	await page.setViewportSize({ width: 390, height: 740 })
	await login(page)

	for (const screen of SCREENS) {
		await open(page, screen)
		const nav = subNav(page, screen.nav)

		await wheelSectionTo(page, screen.jump.hash.slice(1), 185)
		const scrollY = await page.evaluate(() => window.scrollY)
		const height = await page.evaluate(() => document.documentElement.scrollHeight)
		await expect(current(nav)).toHaveText(screen.jump.name)
		// Marcar otra no mueve nada: ni el scroll ni el alto de la página.
		await page.waitForTimeout(300)
		expect(await page.evaluate(() => window.scrollY)).toBe(scrollY)
		expect(await page.evaluate(() => document.documentElement.scrollHeight)).toBe(height)
		expect(await horizontalOverflow(page), `${screen.path}: scroll horizontal`).toBe(0)

		// Un link (la nav está arriba del contenido) salta y marca su sección.
		await wheelTo(page, 0)
		await nav.getByRole('link', { name: screen.short.name }).click()
		await expect(page).toHaveURL(new RegExp(`#${screen.short.id}$`))
		await expect(current(nav)).toHaveText(screen.short.name)
	}

	expect(errors).toEqual([])
})
