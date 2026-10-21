// La distancia al botón y el margen mínimo contra los bordes de la pantalla, en px.
const GAP = 6
const EDGE = 8

// Ubica un panel `position: fixed` junto a su botón y siempre adentro del viewport:
// - abajo si entra; si no, arriba si entra;
// - si no entra de ningún lado (un selector con 20 proyectos en un teléfono), del lado con
//   más lugar, con `max-height` a ese lugar: el panel scrollea adentro.
// Escribe `left`/`top`/`max-height` por CSSOM (lo permite una CSP sin 'unsafe-inline').
export function placeBelow(panel: HTMLElement, anchor: HTMLElement, align: 'start' | 'end'): void {
	// Se mide el alto natural: un `max-height` de una ubicación anterior (antes de un
	// scroll o un resize) lo achicaría.
	panel.style.maxHeight = ''
	const target = anchor.getBoundingClientRect()
	const { width, height } = panel.getBoundingClientRect()
	const viewportWidth = document.documentElement.clientWidth
	const viewportHeight = window.innerHeight

	const preferredLeft = align === 'end' ? target.right - width : target.left
	const maxLeft = Math.max(EDGE, viewportWidth - width - EDGE)
	const left = Math.min(Math.max(preferredLeft, EDGE), maxLeft)

	const below = target.bottom + GAP
	const roomBelow = Math.max(0, viewportHeight - EDGE - below)
	const roomAbove = Math.max(0, target.top - GAP - EDGE)

	let top: number
	if (height <= roomBelow) {
		top = below
	} else if (height <= roomAbove) {
		top = target.top - GAP - height
	} else if (roomBelow >= roomAbove) {
		top = below
		panel.style.maxHeight = `${Math.floor(roomBelow)}px`
	} else {
		top = EDGE
		panel.style.maxHeight = `${Math.floor(roomAbove)}px`
	}

	panel.style.left = `${Math.round(left)}px`
	panel.style.top = `${Math.round(top)}px`
}
