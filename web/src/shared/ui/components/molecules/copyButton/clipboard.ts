import { srOnly } from '@/shared/ui/themes/utilities.css'

// Copia al portapapeles. La API async solo existe en un contexto seguro (HTTPS o
// localhost) y puede estar denegada; si falla, se prueba el camino viejo (un textarea
// fuera de vista + execCommand), que funciona en HTTP dentro de un clic.
export async function writeToClipboard(value: string): Promise<boolean> {
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
	document.body.append(area)
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
