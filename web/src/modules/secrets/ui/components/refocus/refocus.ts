import type { Attachment } from 'svelte/attachments'

/**
 * Para el botón que abre un panel: cuando el panel se cierra con el foco adentro (Cancelar,
 * o después de importar), el panel desaparece y el foco cae en `<body>`. Esto lo devuelve
 * al primer botón del elemento cada vez que `closedCount()` cambia; al montar, no hace nada.
 */
export function refocusOnClose(closedCount: () => number): Attachment<HTMLElement> {
	return (element) => {
		if (closedCount() === 0) return
		// Un cuadro después: en el mismo cambio, el panel todavía no se desmontó y el foco
		// sigue en su botón.
		const frame = requestAnimationFrame(() => {
			const active = document.activeElement
			if (active && active !== document.body && active.isConnected) return
			element.querySelector<HTMLElement>('button')?.focus()
		})
		return () => cancelAnimationFrame(frame)
	}
}

/**
 * Mueve el foco cuando el viewModel lo pide: cuando `seq()` es distinto de 0. Con
 * `button`, al primer botón del elemento (los ⋯ de una fila); si no, al elemento (un
 * título con `tabindex="-1"`). Espera un cuadro: el diálogo que se cierra en el mismo
 * cambio podría mover el foco después. Al terminar avisa con `done(seq)`, para que el
 * viewModel olvide el pedido: si no, cualquier re-render de la fila (un refetch) lo
 * volvería a cumplir y le robaría el foco a otra cosa.
 */
export function focusOnRequest(
	seq: () => number,
	done: (seq: number) => void,
	target: 'self' | 'button' = 'self',
): Attachment<HTMLElement> {
	return (element) => {
		const requested = seq()
		if (requested === 0) return
		const frame = requestAnimationFrame(() => {
			const focusable =
				target === 'button' ? element.querySelector<HTMLElement>('button') : element
			focusable?.focus()
			done(requested)
		})
		return () => cancelAnimationFrame(frame)
	}
}
