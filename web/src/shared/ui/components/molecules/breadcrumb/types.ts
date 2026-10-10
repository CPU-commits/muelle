export interface BreadcrumbItem {
	readonly label: string
	/** Sin href, el segmento es texto (el último, la página actual). */
	readonly href?: string
	/** La inicial en un cuadradito antes del texto (el proyecto). */
	readonly initial?: string
}
