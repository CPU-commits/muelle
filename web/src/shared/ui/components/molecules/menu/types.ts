import type { IconName } from '../../atoms/icon/icons'

// Una entrada de <Menu>: una acción, un link o un separador. `id` es la key del each.
export type MenuEntry =
	| {
			readonly kind: 'action'
			readonly id: string
			readonly label: string
			readonly onSelect: () => void
			readonly icon?: IconName
			/** Rojo, para borrar o revocar. */
			readonly tone?: 'danger'
			readonly disabled?: boolean
	  }
	| {
			readonly kind: 'link'
			readonly id: string
			readonly label: string
			readonly href: string
			readonly icon?: IconName
	  }
	| { readonly kind: 'separator'; readonly id: string }
