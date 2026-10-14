import type { Attachment } from 'svelte/attachments'

/**
 * Dónde deja el foco algo que se cerró sin saber a dónde volver: `<body>`, o el respaldo
 * del Modal de shared (el h1 de `<main>`, o `<main>`) cuando el botón que lo abrió ya no
 * está (el "Importar" del panel, que se desmonta junto con su diálogo de confirmar).
 */
function isFallbackFocus(active: Element | null): boolean {
	if (!active || active === document.body || !active.isConnected) return true
	const main = document.querySelector('main')
	return active === main || (active.tagName === 'H1' && main?.contains(active) === true)
}

/**
 * Para el botón que abre un panel: cuando el panel se cierra con el foco adentro (Cancelar,
 * después de importar, o desde su diálogo de confirmar), el panel desaparece y el foco cae
 * en `<body>` o en el respaldo del Modal. Esto lo devuelve al primer botón del elemento
 * cada vez que `closedCount()` cambia; al montar, no hace nada.
 */
export function refocusOnClose(closedCount: () => number): Attachment<HTMLElement> {
	return (element) => {
		if (closedCount() === 0) return
		// Un cuadro después: en el mismo cambio el panel todavía no se desmontó, y el Modal
		// mueve el foco al cerrarse.
		const frame = requestAnimationFrame(() => {
			if (!isFallbackFocus(document.activeElement)) return
			element.querySelector<HTMLElement>('button')?.focus()
		})
		return () => cancelAnimationFrame(frame)
	}
}

/**
 * Mueve el foco al elemento (un título con `tabindex="-1"`) cuando el viewModel lo pide:
 * cuando `seq()` es distinto de 0. Espera un cuadro: el diálogo que se cierra en el mismo
 * cambio mueve el foco antes. Al terminar avisa con `done(seq)`, para que el viewModel
 * olvide el pedido: si no, un re-render lo volvería a cumplir y le robaría el foco a otra
 * cosa.
 */
export function focusOnRequest(
	seq: () => number,
	done: (seq: number) => void,
): Attachment<HTMLElement> {
	return (element) => {
		const requested = seq()
		if (requested === 0) return
		const frame = requestAnimationFrame(() => {
			element.focus()
			done(requested)
		})
		return () => cancelAnimationFrame(frame)
	}
}
