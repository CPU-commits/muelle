import type { Attachment } from 'svelte/attachments'

/**
 * Llama a `onVisible` una sola vez, cuando el elemento entra en la pantalla (o queda a
 * `margin` de ella). Para no pedir el estado de las filas de Proyectos que no se ven.
 */
export function whenVisible(onVisible: () => void, margin = '200px'): Attachment<HTMLElement> {
	return (element) => {
		const observer = new IntersectionObserver(
			(entries) => {
				if (!entries.some((entry) => entry.isIntersecting)) return
				observer.disconnect()
				onVisible()
			},
			{ rootMargin: `${margin} 0px` },
		)
		observer.observe(element)
		return () => observer.disconnect()
	}
}
