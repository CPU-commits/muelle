import { style } from '@vanilla-extract/css'
import { theme } from '@/shared/ui/themes/theme.css'

export const body = style({
	display: 'flex',
	flexDirection: 'column',
	gap: theme.space[16],
})

// La ficha del deploy al que se vuelve: "Imagen · Desplegado · Configuración".
export const facts = style({
	display: 'grid',
	gridTemplateColumns: 'auto 1fr',
	gap: `${theme.space[10]} ${theme.space[20]}`,
	padding: `${theme.space[14]} ${theme.space[16]}`,
	border: `1px solid ${theme.colors.borders.subtle}`,
	borderRadius: theme.radius.lg,
	fontSize: theme.typography.fontSize[13],
})

export const term = style({
	color: theme.colors.text.tertiary,
})

export const value = style({
	minWidth: 0,
	overflowWrap: 'anywhere',
})

export const note = style({
	color: theme.colors.text.secondary,
	fontSize: theme.typography.fontSize[13],
})
