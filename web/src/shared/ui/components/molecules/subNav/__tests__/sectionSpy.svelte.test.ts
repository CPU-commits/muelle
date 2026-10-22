import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { SectionSpy } from '../sectionSpy.svelte.ts'
import {
	above,
	atBottom,
	band,
	bottom,
	crossing,
	installFakeObserver,
	mutationsSeen,
	passed,
	section,
	sectionEl,
	sectionsPage,
} from './fakeIntersectionObserver'

// La marca de un link y la de un deep link, sobre lo que dice watchSections (con el
// IntersectionObserver de mentira de fakeIntersectionObserver.ts).

const IDS = ['a', 'b', 'c'] as const

let page: ReturnType<typeof sectionsPage>
let stops: (() => void)[] = []

/** Monta el spy sobre el contenedor (lo que hace `{@attach spy.track}`). */
function track(spy: SectionSpy): void {
	const stop = spy.track(page.container)
	if (typeof stop === 'function') stops.push(stop)
}

/**
 * Las secciones que tocan la franja de arriba de la pantalla; todas menos la última ya
 * empezaron más arriba (como al scrollear hacia abajo): a la vista está la última.
 */
const fire = (...touching: string[]) => {
	above().fire(passed(IDS, ...touching.slice(0, -1)))
	band().fire(crossing(IDS, ...touching))
}

/** Quien lee toca la página (la rueda, por ejemplo). */
const touch = (type = 'wheel') => document.body.dispatchEvent(new Event(type, { bubbles: true }))

/** Espera a que pase el salto de un link (dos frames). */
const settled = () =>
	new Promise<void>((resolve) =>
		requestAnimationFrame(() => requestAnimationFrame(() => resolve())),
	)

describe('SectionSpy', () => {
	beforeEach(() => {
		installFakeObserver()
		page = sectionsPage(IDS)
	})

	afterEach(() => {
		for (const stop of stops) stop()
		stops = []
		page.remove()
		vi.unstubAllGlobals()
		vi.restoreAllMocks()
	})

	it('sin link ni ancla, la activa es la sección a la vista', () => {
		const spy = new SectionSpy(IDS)
		expect(spy.current).toBeNull()
		track(spy)

		fire('a', 'b')
		expect(spy.current).toBe('b')

		window.scrollTo(0, 500)
		bottom().fire(atBottom(true))
		expect(spy.current).toBe('c')
	})

	it('elegir un link la marca enseguida y no parpadea mientras el salto pasa por otras', async () => {
		const spy = new SectionSpy(IDS)
		track(spy)
		fire('a')

		spy.select('c')
		expect(spy.current).toBe('c')

		// El salto pasa por "b" y termina sin que "c" llegue arriba (la página no da para más).
		fire('b')
		expect(spy.current).toBe('c')
		await settled()
		expect(spy.current).toBe('c')

		// Algo que no cambia la sección a la vista (otra entrada igual) no la suelta.
		fire('b')
		expect(spy.current).toBe('c')

		// Que cambie lo que se ve sin que cambie el contenido sí (quien lee scrolleó, o volvió
		// atrás en el historial): la activa vuelve a seguir al scroll.
		fire('a')
		expect(spy.current).toBe('a')
	})

	it('el salto se mide con lo que el navegador ya calculó aunque todavía no lo entregó', async () => {
		const spy = new SectionSpy(IDS)
		track(spy)
		fire('a')

		spy.select('b')
		// El frame del salto calculó "c", pero la entrada llega tarde (una tarea larga).
		band().queue(crossing(IDS, 'c'))
		await settled()
		expect(spy.current).toBe('b')

		fire('c')
		expect(spy.current).toBe('b')
		fire('a')
		expect(spy.current).toBe('a')
	})

	it('al fondo, elegir una sección que no es la última la deja marcada', async () => {
		const spy = new SectionSpy(IDS)
		track(spy)
		window.scrollTo(0, 500)
		fire('b')
		bottom().fire(atBottom(true))
		expect(spy.current).toBe('c')

		spy.select('b')
		await settled()
		expect(spy.current).toBe('b')

		// Subir un poco: deja de estar al fondo y sigue al scroll.
		fire('a')
		bottom().fire(atBottom(false))
		expect(spy.current).toBe('a')
	})

	it('elegir un link que no es una sección suelta la marca', () => {
		const spy = new SectionSpy(IDS)
		track(spy)
		fire('a')

		spy.select('c')
		spy.select(null)
		expect(spy.current).toBe('a')

		spy.select('c')
		spy.select('otra')
		expect(spy.current).toBe('a')
	})

	it('elegir una sección que todavía no está (viene de otra página) la trae cuando aparece', async () => {
		sectionEl('c').remove()
		const spy = new SectionSpy(IDS)
		track(spy)
		// La página de la que viene el link estaba scrolleada (Tokens).
		window.scrollTo(0, 300)

		spy.select('c')
		expect(spy.current).toBe('c')
		// Mientras llega, lo que se vea (la página de antes, scrolleada) no la suelta.
		fire('a')
		expect(spy.current).toBe('c')

		const late = section('c')
		const scrollIntoView = vi.spyOn(late, 'scrollIntoView')
		page.container.append(late)
		await mutationsSeen()

		expect(scrollIntoView).toHaveBeenCalledOnce()
		await settled()
		expect(spy.current).toBe('c')
	})

	it('un deep link marca su sección desde el principio y la trae a la vista', () => {
		const spy = new SectionSpy(IDS, { initial: 'b' })
		expect(spy.current).toBe('b')

		const scrollIntoView = vi.spyOn(sectionEl('b'), 'scrollIntoView')
		track(spy)

		expect(scrollIntoView).toHaveBeenCalledOnce()
		fire('b', 'c')
		expect(spy.current).toBe('b')
	})

	it('si la sección del deep link aparece después (carga aparte), la trae cuando aparece', async () => {
		sectionEl('b').remove()
		const spy = new SectionSpy(IDS, { initial: 'b' })
		track(spy)
		fire()
		expect(spy.current).toBe('b')

		const late = section('b')
		const scrollIntoView = vi.spyOn(late, 'scrollIntoView')
		sectionEl('a').after(late)
		await mutationsSeen()

		expect(scrollIntoView).toHaveBeenCalledOnce()
		expect(spy.current).toBe('b')
	})

	it('si quien lee toca la página antes de que aparezca la sección del deep link, no salta y la suelta', async () => {
		sectionEl('b').remove()
		const spy = new SectionSpy(IDS, { initial: 'b' })
		track(spy)

		touch()
		window.scrollTo(0, 300)
		fire('a')
		expect(spy.current).toBe('a')

		const late = section('b')
		const scrollIntoView = vi.spyOn(late, 'scrollIntoView')
		sectionEl('a').after(late)
		await mutationsSeen()

		expect(scrollIntoView).not.toHaveBeenCalled()
		expect(spy.current).toBe('a')
	})

	it('con la página scrolleada al montar (se llegó desde otra página), el deep link igual la trae', () => {
		window.scrollTo(0, 300)
		const spy = new SectionSpy(IDS, { initial: 'b' })
		const scrollIntoView = vi.spyOn(sectionEl('b'), 'scrollIntoView')
		track(spy)

		expect(scrollIntoView).toHaveBeenCalledOnce()
		fire('a')
		expect(spy.current).toBe('b')
	})

	it('lo que llega después arriba de un deep link no lo suelta: lo vuelve a traer a la vista', async () => {
		const spy = new SectionSpy(IDS, { initial: 'b' })
		const scrollIntoView = vi.spyOn(sectionEl('b'), 'scrollIntoView')
		track(spy)
		await settled()
		expect(scrollIntoView).toHaveBeenCalledOnce()

		// Llega una sección de arriba con su query: lo que se ve cambia sin que nadie scrollee.
		const loaded = document.createElement('div')
		loaded.style.height = '2000px'
		page.container.prepend(loaded)
		await mutationsSeen()
		fire('a')

		expect(scrollIntoView.mock.calls.length).toBeGreaterThan(1)
		expect(spy.current).toBe('b')
		await settled()
		expect(spy.current).toBe('b')

		// Una que crece (sin elementos nuevos: una imagen, una fuente) tampoco.
		scrollIntoView.mockClear()
		loaded.style.height = '2400px'
		await settled()
		expect(scrollIntoView).toHaveBeenCalledOnce()
		await settled()
		expect(spy.current).toBe('b')

		// Quien lee scrollea: la activa vuelve a seguir al scroll.
		touch()
		fire('c')
		expect(spy.current).toBe('c')
	})

	it('después de que quien lee toca la página, un cambio del contenido ya no trae la sección ni la sostiene', async () => {
		const spy = new SectionSpy(IDS)
		track(spy)
		fire('a')
		spy.select('c')
		await settled()

		const scrollIntoView = vi.spyOn(sectionEl('c'), 'scrollIntoView')
		touch('pointerdown')
		// Lo que tocó cambió el contenido (otra opción en Healthcheck): lo que se ve es otra.
		page.container.prepend(document.createElement('p'))
		await mutationsSeen()
		fire('b')

		expect(scrollIntoView).not.toHaveBeenCalled()
		expect(spy.current).toBe('b')
	})

	it('si la sección marcada se va de la página (se volvió a otra), suelta la marca', async () => {
		const spy = new SectionSpy(IDS)
		track(spy)
		fire('a')
		spy.select('c')
		await settled()

		sectionEl('c').remove()
		await mutationsSeen()

		expect(spy.current).toBe('a')
	})

	it('un ancla que no es una de las secciones no marca nada', () => {
		const spy = new SectionSpy(IDS, { initial: 'otra' })
		const scrollIntoView = vi.spyOn(sectionEl('a'), 'scrollIntoView')
		track(spy)

		expect(spy.current).toBeNull()
		expect(scrollIntoView).not.toHaveBeenCalled()
	})

	it('al desmontarse deja de observar y no queda ninguna activa', () => {
		const spy = new SectionSpy(IDS)
		track(spy)
		const observers = [band(), above(), bottom()]
		fire('a')

		for (const stop of stops) stop()
		stops = []

		expect(observers.every((observer) => observer.disconnected)).toBe(true)
		expect(spy.current).toBeNull()
	})
})
