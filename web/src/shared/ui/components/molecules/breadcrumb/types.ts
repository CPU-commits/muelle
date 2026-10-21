import type { MenuEntry } from '../menu/types'

/** Un segmento de link (o de texto, sin `href`: la página actual). */
export interface BreadcrumbLink {
	readonly kind?: 'link'
	readonly label: string
	/** Sin href, el segmento es texto (el último, la página actual). */
	readonly href?: string
	/** La inicial en un cuadradito antes del texto (el proyecto). */
	readonly initial?: string
}

/**
 * El selector del diseño ("auditfy ⇅"): un botón con chevron que abre un <Menu> para
 * cambiar de proyecto, stage o app.
 */
export interface BreadcrumbMenu {
	readonly kind: 'menu'
	readonly label: string
	/** El nombre del botón: "Proyecto auditfy, cambiar". */
	readonly ariaLabel: string
	readonly items: readonly MenuEntry[]
	readonly initial?: string
}

export type BreadcrumbItem = BreadcrumbLink | BreadcrumbMenu
