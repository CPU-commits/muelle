import { untrack } from 'svelte'
import type { Attachment } from 'svelte/attachments'
import { type SectionWatch, watchSections } from './watchSections'

// La sección activa de una página con nav de secciones (SubNavLayout): la que se está leyendo
// (watchSections, con IntersectionObserver y sin escuchar el scroll), salvo que la haya
// marcado un link o un deep link:
// - un link de la nav (`select`) la marca en el acto y la deja marcada mientras dura el salto
//   (no parpadea con las secciones por las que pasa) y hasta que se scrollea a mano: lo que
//   muestra el scroll cambia respecto de donde terminó el salto;
// - un deep link (`initial`) marca su sección desde el principio y, si la página la dibuja
//   después de que SvelteKit buscó el ancla (llega con su query), la trae a la vista.
// El hash de la URL no se toca: lo pone el link al hacer clic y nada más. No hay scroll suave:
// los saltos son los del navegador, instantáneos (nada que frenar con prefers-reduced-motion).

// Mientras dura el salto de un link no hay contra qué comparar.
const SETTLING = Symbol('settling')

export interface SectionSpyOptions {
	/** El ancla con la que se abrió la página (sin `#`), o null. Se lee una sola vez. */
	readonly initial?: string | null
}

export class SectionSpy {
	readonly #ids: readonly string[]
	#observed = $state<string | null>(null)
	#pinned = $state<string | null>(null)
	// Lo que mostraba el scroll al terminar el salto: mientras siga igual, la marca se queda.
	#baseline: string | null | typeof SETTLING = SETTLING
	// La sección marcada todavía no está en la página: traerla cuando aparezca.
	#pending = false
	#settleRun = 0
	#watch: SectionWatch | null = null

	/** `ids`: las secciones de la página (sus `id`), en el orden de la nav y de la página. */
	constructor(ids: readonly string[], { initial = null }: SectionSpyOptions = {}) {
		this.#ids = ids
		if (initial !== null && ids.includes(initial)) {
			this.#pinned = initial
			this.#pending = true
		}
	}

	/** La sección activa, o null si todavía no se llegó a ninguna (arriba de todo). */
	get current(): string | null {
		return this.#pinned ?? this.#observed
	}

	/**
	 * Se siguió en esta pestaña un link de la nav que lleva a `id` (el salto y el hash son del
	 * link). Con algo que no es una de las secciones (`null`, otra página), suelta la marca.
	 */
	select = (id: string | null): void => {
		if (id === null || !this.#ids.includes(id)) {
			this.#unpin()
			return
		}
		this.#pinned = id
		this.#pending = this.#watch?.element(id) === undefined
		if (this.#pending) {
			this.#settleRun += 1
			this.#baseline = SETTLING
		} else {
			this.#settle()
		}
	}

	/** Para `{@attach}` en el elemento que tiene las secciones adentro. */
	track: Attachment<HTMLElement> = (container) =>
		untrack(() => {
			const watch = watchSections(container, this.#ids, () => this.#update())
			this.#watch = watch
			this.#update()
			return () => {
				watch.stop()
				this.#watch = null
				this.#observed = null
			}
		})

	#update(): void {
		const watch = this.#watch
		if (watch === null) return
		this.#observed = watch.inView
		this.#arrive(watch)
		this.#review()
	}

	// La sección marcada que faltaba ya está: traerla a la vista, salvo que alguien haya movido
	// la página mientras tanto (quien lee, o SvelteKit al recuperar el scroll de un F5).
	#arrive(watch: SectionWatch): void {
		const id = this.#pinned
		if (!this.#pending || id === null) return
		const element = watch.element(id)
		if (element === undefined) return
		this.#pending = false
		if (window.scrollY > 0) {
			this.#unpin()
			return
		}
		element.scrollIntoView()
		this.#settle()
	}

	// El salto termina antes del frame siguiente: ahí se anota lo que muestra el scroll, con lo
	// que el navegador ya calculó aunque todavía no lo haya entregado.
	#settle(): void {
		this.#baseline = SETTLING
		const run = ++this.#settleRun
		requestAnimationFrame(() =>
			requestAnimationFrame(() => {
				if (run !== this.#settleRun) return
				this.#watch?.flush()
				this.#baseline = this.#observed
			}),
		)
	}

	// Después de cada cambio de lo que se ve: ¿sigue la marca del link?
	#review(): void {
		if (this.#pinned === null) return
		if (this.#pending) {
			// Su sección todavía no llegó: solo la suelta que se haya scrolleado.
			if (window.scrollY > 0) this.#unpin()
			return
		}
		if (this.#baseline !== SETTLING && this.#observed !== this.#baseline) this.#unpin()
	}

	#unpin(): void {
		this.#pinned = null
		this.#pending = false
		this.#settleRun += 1
	}
}
