import { style } from '@vanilla-extract/css'
import { theme } from '@/shared/ui/themes/theme.css'

// El encabezado de las secciones de lista de Ajustes (Registries, Auditoría): título y
// texto a la izquierda, las acciones a la derecha.
export const heading = style({
	display: 'flex',
	flexWrap: 'wrap',
	alignItems: 'center',
	justifyContent: 'space-between',
	gap: `${theme.space[8]} ${theme.space[16]}`,
})

export const text = style({
	display: 'flex',
	flexDirection: 'column',
	gap: theme.space[2],
})

export const title = style({
	fontSize: theme.typography.fontSize[16],
	fontWeight: theme.typography.fontWeight.semibold,
	letterSpacing: theme.typography.letterSpacing.snug,
})

export const description = style({
	fontSize: theme.typography.fontSize[13],
	color: theme.colors.text.tertiary,
})

export const actions = style({
	display: 'flex',
	flexWrap: 'wrap',
	gap: theme.space[8],
})
