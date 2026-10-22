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

// El nombre ocupa toda la fila: es lo primero que se escribe y puede ser largo. Permisos
// también, con su texto de ayuda.
export const wide = style({
	gridColumn: '1 / -1',
})

// "Permisos": un grupo con la casilla "Solo lectura" y su ayuda, con el ritmo de un Field
// (la leyenda como su label, la ayuda como su mensaje).
export const access = style({
	display: 'flex',
	flexDirection: 'column',
	alignItems: 'flex-start',
	gap: theme.space[6],
	minWidth: 0,
	margin: 0,
	padding: 0,
	border: 'none',
})

export const accessLegend = style({
	padding: 0,
	marginBottom: theme.space[2],
	fontSize: theme.typography.fontSize[13],
	fontWeight: theme.typography.fontWeight.medium,
	color: theme.colors.text.secondary,
})

export const accessHint = style({
	maxWidth: '68ch',
	fontSize: theme.typography.fontSize[13],
	color: theme.colors.text.tertiary,
})
