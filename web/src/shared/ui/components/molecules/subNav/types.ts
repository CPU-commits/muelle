import type { NavItem } from '../tabs/types'

// Un link de la SubNav. `onselect` se llama cuando el link se sigue en esta pestaña (un clic
// sin teclas que lo manden a otra): con un SectionSpy, `() => spy.select(id)` marca la sección
// en el acto. El salto y el hash siguen siendo del link.
export interface SubNavItem extends NavItem {
	readonly onselect?: () => void
}
