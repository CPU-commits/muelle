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

export interface SectionWatch {
	/** La sección a la vista, o null si ninguna llegó a la franja todavía (arriba de todo). */
	readonly inView: string | null
	/** El elemento de la sección `id`, si está en la página. */
	element(id: string): Element | undefined
	/** Aplica lo que el navegador ya calculó y todavía no entregó (avisa con `onChange`). */
	flush(): void
	stop(): void
}

/**
 * Busca las secciones `ids` (por su `id`) adentro de `container` y llama a `onChange` cada vez
 * que puede haber cambiado `inView` o las secciones que hay. Al empezar no avisa: el estado
 * inicial se lee de lo que devuelve.
 */
export function watchSections(
	container: HTMLElement,
	ids: readonly string[],
	onChange: () => void,
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
		onChange()
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
		onChange()
	})
	mutations.observe(container, { childList: true, subtree: true })
	resolve()

	return {
		get inView() {
			return inView
		},
		element: (id) => elements.get(id),
		flush: delivered,
		stop: () => {
			band.disconnect()
			above.disconnect()
			bottom.disconnect()
			mutations.disconnect()
		},
	}
}
