// Qué sección de una página con nav de secciones (SubNavLayout) se está leyendo, con
// IntersectionObserver y sin escuchar el scroll. Es la parte de SectionSpy que mira el DOM;
// la marca de un link y el deep link son de SectionSpy.
// - A la vista está la que ocupa la parte de arriba de la pantalla (una franja del 30 % de
//   arriba): la primera que empieza en la franja (su borde de arriba está a la vista), aunque
//   sea corta y la siguiente ya asome debajo (una tarjeta a la altura de la nav, donde la
//   deja un salto); si ninguna empieza ahí, la que viene de más arriba y la ocupa.
// - Al fondo de la página gana la última, aunque sea corta y nunca llegue arriba
//   ("Eliminar app").
// - Las secciones pueden aparecer después (cada una con su query) o irse: un
//   MutationObserver sobre el contenedor las vuelve a buscar.
// - Avisa por separado lo que movió el scroll (`view`), lo que cambió el contenido (`layout`:
//   secciones que llegan o se van, un alto que cambia) y que quien lee tocó la página
//   (`input`): SectionSpy necesita saber quién movió lo que se ve.
// La raíz de los observers es el documento (`root: document`): su pantalla, la misma que
// la implícita en la app. Adentro de un iframe (el de los tests de componentes, escalado
// dentro de la página de vitest), la implícita sería la pantalla de la página de afuera.

// La franja de arriba de la pantalla: el 30 %.
const BAND = '0px 0px -70% 0px'
// Todo lo que está por encima del borde de arriba de la pantalla: una sección que lo toca ya
// empezó más arriba (su borde de arriba pasó el de la pantalla).
const ABOVE = '1000000px 0px -100% 0px'
// El fondo de la página: el documento entero adentro de la pantalla estirada hacia arriba
// (lo que ya pasó cuenta como adentro), con `threshold: 1`. Es lo que avisa justo cuando el
// borde de abajo del documento llega al de la pantalla. Con un píxel de holgura abajo: con un
// alto con decimales (1889.4375 px), al fondo el documento termina una fracción de píxel por
// debajo de la pantalla y nunca entraría entero.
const STRETCHED_UP = '1000000px 0px 1px 0px'

// Lo que hace quien lee y puede mover la página: la rueda, el dedo, una tecla (las que
// scrollean, Tab, buscar en la página) y el puntero (también en la barra de scroll). Son
// eventos de entrada, no del scroll: un scroll que hace la página (un salto, el historial) no
// los dispara.
const INPUT_EVENTS = ['wheel', 'touchmove', 'keydown', 'pointerdown'] as const

export interface SectionEvents {
	/** El scroll movió las secciones respecto de la franja o del fondo: `inView` pudo cambiar. */
	readonly view: () => void
	/**
	 * Cambió el contenido: llegaron o se fueron secciones, o cambió el alto de algo. `inView`
	 * también pudo cambiar (y lo que el navegador calcule de ahí llega después con `view`).
	 */
	readonly layout: () => void
	/** Quien lee tocó la página: la rueda, el dedo, una tecla o el puntero. */
	readonly input: () => void
}

export interface SectionWatch {
	/** La sección a la vista, o null si ninguna llegó a la franja todavía (arriba de todo). */
	readonly inView: string | null
	/** El elemento de la sección `id`, si está en la página. */
	element(id: string): Element | undefined
	/** Aplica lo que el navegador ya calculó y todavía no entregó, sin avisar: se lee `inView`. */
	flush(): void
	stop(): void
}

/**
 * Busca las secciones `ids` (por su `id`) adentro de `container` y avisa con `events` lo que
 * puede haber cambiado `inView` o las secciones que hay. Al empezar no avisa: el estado inicial
 * se lee de lo que devuelve.
 */
export function watchSections(
	container: HTMLElement,
	ids: readonly string[],
	events: SectionEvents,
): SectionWatch {
	const elements = new Map<string, Element>()
	const idOf = new WeakMap<Element, string>()
	// Las que tocan la franja y las que ya empezaron más arriba de la pantalla.
	const touching = new Set<string>()
	const started = new Set<string>()
	let atBottom = false
	let inView: string | null = null

	const apply = (set: Set<string>, entries: IntersectionObserverEntry[]) => {
		for (const entry of entries) {
			const id = idOf.get(entry.target)
			if (id === undefined || elements.get(id) !== entry.target) continue
			if (entry.isIntersecting) set.add(id)
			else set.delete(id)
		}
	}
	// Una página que no scrollea también tiene el documento entero a la vista: eso no es
	// haber bajado hasta el fondo.
	const applyBottom = (entries: IntersectionObserverEntry[]) => {
		const last = entries.at(-1)
		if (last) atBottom = last.intersectionRatio >= 1 && window.scrollY > 0
	}
	const compute = () => {
		const present = ids.filter((id) => elements.has(id))
		const last = present.at(-1) ?? null
		const inBand = present.filter((id) => touching.has(id))
		const first = inBand.find((id) => !started.has(id))
		inView = atBottom && last !== null ? last : (first ?? inBand.at(-1) ?? null)
	}

	// El navegador llama a cada observer por separado, aunque los haya calculado en el mismo
	// frame: el primero se lleva también lo que los otros todavía no entregaron, así nunca se
	// mira una mitad (una sección intermedia que la nav marcaría por un instante).
	const takeAll = () => {
		apply(touching, band.takeRecords())
		apply(started, above.takeRecords())
		applyBottom(bottom.takeRecords())
	}
	const delivered = () => {
		takeAll()
		compute()
		events.view()
	}

	const band = new IntersectionObserver(
		(entries) => {
			apply(touching, entries)
			delivered()
		},
		{ root: document, rootMargin: BAND },
	)
	const above = new IntersectionObserver(
		(entries) => {
			apply(started, entries)
			delivered()
		},
		{ root: document, rootMargin: ABOVE },
	)
	const bottom = new IntersectionObserver(
		(entries) => {
			applyBottom(entries)
			delivered()
		},
		{ root: document, rootMargin: STRETCHED_UP, threshold: 1 },
	)
	bottom.observe(document.documentElement)

	const resolve = () => {
		for (const id of ids) {
			const element = container.querySelector(`#${CSS.escape(id)}`)
			const previous = elements.get(id)
			if (element === previous) continue
			if (previous) {
				band.unobserve(previous)
				above.unobserve(previous)
				elements.delete(id)
				touching.delete(id)
				started.delete(id)
			}
			if (element) {
				elements.set(id, element)
				idOf.set(element, id)
				band.observe(element)
				above.observe(element)
			}
		}
		compute()
	}
	const mutations = new MutationObserver(() => {
		resolve()
		events.layout()
	})
	mutations.observe(container, { childList: true, subtree: true })
	resolve()

	// El alto (o el ancho) del contenido cambia también sin elementos nuevos: una imagen o una
	// fuente que llega, una clase que muestra algo. El primer aviso es el tamaño con el que
	// empieza, no un cambio.
	let size: string | null = null
	const resizes = new ResizeObserver(([entry]) => {
		if (entry === undefined) return
		const next = `${entry.contentRect.width}x${entry.contentRect.height}`
		const changed = size !== null && next !== size
		size = next
		if (changed) events.layout()
	})
	resizes.observe(container)

	const input = () => events.input()
	const listening = { capture: true, passive: true } as const
	for (const type of INPUT_EVENTS) window.addEventListener(type, input, listening)

	return {
		get inView() {
			return inView
		},
		element: (id) => elements.get(id),
		flush: () => {
			takeAll()
			compute()
		},
		stop: () => {
			band.disconnect()
			above.disconnect()
			bottom.disconnect()
			mutations.disconnect()
			resizes.disconnect()
			for (const type of INPUT_EVENTS) window.removeEventListener(type, input, listening)
		},
	}
}
