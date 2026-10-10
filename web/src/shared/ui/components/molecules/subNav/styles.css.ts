import { style } from '@vanilla-extract/css'
import { theme } from '@/shared/ui/themes/theme.css'

// La nav vertical de Ajustes y de Configuración (`m-sub` en el diseño).
export const nav = style({
	display: 'flex',
	flexDirection: 'column',
	gap: theme.space[2],
	minWidth: 0,
})

export const link = style({
	display: 'flex',
	alignItems: 'center',
	height: theme.control.sm,
	paddingInline: theme.space[10],
	borderRadius: theme.radius.md,
	color: theme.colors.text.secondary,
	transition: `color ${theme.transition.fast}, background-color ${theme.transition.fast}`,
	selectors: {
		'&:hover': { color: theme.colors.text.primary },
	},
})

export const linkActive = style({
	backgroundColor: theme.colors.background.raised,
	color: theme.colors.text.primary,
	fontWeight: theme.typography.fontWeight.medium,
})

// El layout de dos columnas que acompaña a la subnav: 180 px de nav y el resto de
// contenido; en un teléfono, una debajo de la otra.
export const layout = style({
	display: 'flex',
	flexWrap: 'wrap',
	alignItems: 'flex-start',
	gap: `${theme.space[24]} ${theme.space[40]}`,
})

export const aside = style({
	flex: '1 1 180px',
	minWidth: 0,
})

export const content = style({
	flex: '4 1 600px',
	minWidth: 0,
	display: 'flex',
	flexDirection: 'column',
	gap: theme.space[20],
})
