import { style } from '@vanilla-extract/css'
import { theme } from '@/shared/ui/themes/theme.css'

// Las secciones de la página van a 24 px, como en el diseño.
export const page = style({
	display: 'flex',
	flexDirection: 'column',
	gap: theme.space[24],
	minWidth: 0,
})
