import type { Attachment } from 'svelte/attachments'

/**
 * Para el botón que abre un panel: cuando el panel se cierra con el foco adentro (Cancelar,
 * o después de importar), el panel desaparece y el foco cae en `<body>`. Esto lo devuelve
 * al primer botón del elemento cada vez que `closedCount()` cambia; al montar, no hace nada.
 */
export function refocusOnClose(closedCount: () => number): Attachment<HTMLElement> {
	return (element) => {
		if (closedCount() === 0) return
		const active = document.activeElement
		if (active && active !== document.body && active.isConnected) return
		element.querySelector<HTMLElement>('button')?.focus()
	}
}

/**
 * Mueve el foco cuando el viewModel lo pide: cada vez que `seq()` cambia a un valor
 * distinto de 0. Con `button`, al primer botón del elemento (los ⋯ de una fila); si no, al
 * elemento (un título con `tabindex="-1"`). Espera un cuadro: el diálogo que se cierra en
 * el mismo cambio podría mover el foco después.
 */
export function focusOnRequest(
	seq: () => number,
	target: 'self' | 'button' = 'self',
): Attachment<HTMLElement> {
	return (element) => {
		if (seq() === 0) return
		const frame = requestAnimationFrame(() => {
			const focusable =
				target === 'button' ? element.querySelector<HTMLElement>('button') : element
			focusable?.focus()
		})
		return () => cancelAnimationFrame(frame)
	}
}
