// La distancia al botón y el margen mínimo contra los bordes de la pantalla, en px.
const GAP = 6
const EDGE = 8

// Ubica un panel `position: fixed` junto a su botón: abajo; arriba si abajo no entra y
// arriba sí; y siempre adentro del viewport. Escribe `left`/`top` por CSSOM (lo permite
// una CSP sin 'unsafe-inline').
export function placeBelow(panel: HTMLElement, anchor: HTMLElement, align: 'start' | 'end'): void {
	const target = anchor.getBoundingClientRect()
	const { width, height } = panel.getBoundingClientRect()
	const viewportWidth = document.documentElement.clientWidth
	const viewportHeight = window.innerHeight

	const preferredLeft = align === 'end' ? target.right - width : target.left
	const maxLeft = Math.max(EDGE, viewportWidth - width - EDGE)
	const left = Math.min(Math.max(preferredLeft, EDGE), maxLeft)

	const below = target.bottom + GAP
	const above = target.top - GAP - height
	const fitsBelow = below + height <= viewportHeight - EDGE
	const top = fitsBelow || above < EDGE ? below : above

	panel.style.left = `${Math.round(left)}px`
	panel.style.top = `${Math.round(top)}px`
}
