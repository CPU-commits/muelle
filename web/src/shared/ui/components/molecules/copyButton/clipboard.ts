import { srOnly } from '@/shared/ui/themes/utilities.css'

// Copia al portapapeles. La API async solo existe en un contexto seguro (HTTPS o
// localhost) y puede estar denegada; si falla, se prueba el camino viejo (un textarea
// fuera de vista + execCommand), que funciona en HTTP dentro de un clic.
//
// `near` es el botón que copia: si está adentro de un <dialog> modal, el textarea va
// adentro del <dialog>. Con un modal abierto el resto de la página está inerte, y un
// textarea en el <body> no se puede seleccionar: execCommand no copiaría nada.
export async function writeToClipboard(value: string, near?: Element | null): Promise<boolean> {
	try {
		if (navigator.clipboard?.writeText) {
			await navigator.clipboard.writeText(value)
			return true
		}
	} catch {
		// Sigue con el plan B.
	}
	const area = document.createElement('textarea')
	area.value = value
	area.readOnly = true
	area.className = srOnly
	const previous = document.activeElement instanceof HTMLElement ? document.activeElement : null
	const container = near?.closest('dialog[open]') ?? document.body
	container.append(area)
	try {
		area.select()
		return document.execCommand('copy')
	} catch {
		return false
	} finally {
		area.remove()
		previous?.focus()
	}
}

// Deja seleccionado el texto de `id` para que se copie a mano (Ctrl+C) cuando no se pudo.
export function selectText(id: string): void {
	const element = document.getElementById(id)
	if (!element) return
	if (element instanceof HTMLInputElement || element instanceof HTMLTextAreaElement) {
		element.focus()
		element.select()
		return
	}
	const range = document.createRange()
	range.selectNodeContents(element)
	const selection = window.getSelection()
	selection?.removeAllRanges()
	selection?.addRange(range)
}
