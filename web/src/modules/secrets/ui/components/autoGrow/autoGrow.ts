import type { Attachment } from 'svelte/attachments'

/**
 * Un `<textarea>` de una línea que crece con su contenido (un valor pegado de varias
 * líneas, como una clave PEM), hasta el `max-height` de su CSS. `value` es el valor que
 * muestra: cuando el viewModel lo cambia (lo limpia después de agregar), también se ajusta.
 */
export function autoGrow(value: () => string): Attachment<HTMLTextAreaElement> {
	return (element) => {
		value()
		const resize = () => {
			element.style.height = 'auto'
			const borders = element.offsetHeight - element.clientHeight
			element.style.height = `${element.scrollHeight + borders}px`
		}
		// Después de que Svelte escribió el valor nuevo en el elemento.
		queueMicrotask(resize)
		element.addEventListener('input', resize)
		return () => element.removeEventListener('input', resize)
	}
}
