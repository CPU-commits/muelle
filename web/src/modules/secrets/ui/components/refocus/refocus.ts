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
