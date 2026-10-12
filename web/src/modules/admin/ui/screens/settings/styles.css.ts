import { style } from '@vanilla-extract/css'
import { theme } from '@/shared/ui/themes/theme.css'

// Las secciones van una debajo de la otra, con el mismo aire que el diseño (20 px).
export const screen = style({
	display: 'flex',
	flexDirection: 'column',
	gap: theme.space[20],
	minWidth: 0,
})
