// Un link de navegación (Tabs, SubNav). `active` lo calcula quien arma la lista, con la
// ruta actual: el componente no conoce el router.
export interface NavItem {
	readonly href: string
	readonly label: string
	readonly active: boolean
}
