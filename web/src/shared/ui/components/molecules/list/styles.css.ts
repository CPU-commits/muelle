import { style, styleVariants } from '@vanilla-extract/css'
import { theme } from '@/shared/ui/themes/theme.css'

// La lista con borde del diseño (apps de un stage, variables, tokens, usuarios): las
// filas se separan con 1 px del color `divider` que asoma por el `gap`.
export const list = style({
	listStyle: 'none',
	margin: 0,
	padding: 0,
	display: 'flex',
	flexDirection: 'column',
	gap: '1px',
	border: `1px solid ${theme.colors.borders.subtle}`,
	borderRadius: theme.radius.lg,
	backgroundColor: theme.colors.background.divider,
	overflow: 'hidden',
})

export const row = style({
	display: 'flex',
	flexWrap: 'wrap',
	alignItems: 'center',
	gap: `${theme.space[4]} ${theme.space[16]}`,
	boxSizing: 'border-box',
	padding: `${theme.space[8]} ${theme.space[8]} ${theme.space[8]} ${theme.space[20]}`,
	backgroundColor: theme.colors.background.surface,
	color: theme.colors.text.primary,
	transition: `background-color ${theme.transition.fast}`,
})

export const interactive = style({
	selectors: {
		'&:hover': { backgroundColor: theme.colors.background.hover },
	},
})

export const muted = style({
	backgroundColor: theme.colors.background.sunken,
})

export const heights = styleVariants({
	sm: { minHeight: '44px' },
	md: { minHeight: '52px' },
	lg: { minHeight: '56px' },
})

export const item = style({
	display: 'block',
})

export type ListRowHeight = keyof typeof heights
