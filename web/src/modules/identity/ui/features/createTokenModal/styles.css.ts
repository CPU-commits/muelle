import { style } from '@vanilla-extract/css'
import { theme } from '@/shared/ui/themes/theme.css'

export const form = style({
	display: 'flex',
	flexDirection: 'column',
	gap: theme.space[16],
})

export const fields = style({
	display: 'grid',
	gridTemplateColumns: 'repeat(auto-fit, minmax(min(170px, 100%), 1fr))',
	gap: theme.space[16],
})

// El nombre ocupa toda la fila: es lo primero que se escribe y puede ser largo.
export const wide = style({
	gridColumn: '1 / -1',
})
