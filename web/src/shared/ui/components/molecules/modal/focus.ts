// A dónde va el foco cuando se cierra un diálogo: al elemento que lo tenía antes de
// abrirlo; si ya no está (se borró la fila que tenía el botón), al título de la página
// (el h1 de <main>) o a <main>. Nunca se queda en <body>, donde un lector de pantalla
// pierde el hilo.
export function restoreFocus(previous: HTMLElement | null): void {
	if (previous?.isConnected && !previous.closest('[inert]')) {
		previous.focus()
		if (document.activeElement === previous) return
	}
	const main = document.querySelector('main')
	const fallback = main?.querySelector<HTMLElement>('h1') ?? main
	if (!fallback) return
	if (!fallback.hasAttribute('tabindex')) fallback.setAttribute('tabindex', '-1')
	fallback.focus()
}
