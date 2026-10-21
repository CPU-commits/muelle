import { style, styleVariants } from '@vanilla-extract/css'
import { theme } from '@/shared/ui/themes/theme.css'

export const header = style({
	display: 'flex',
	flexWrap: 'wrap',
	alignItems: 'center',
	justifyContent: 'space-between',
	gap: `${theme.space[12]} ${theme.space[16]}`,
})

export const text = style({
	display: 'flex',
	flexDirection: 'column',
	gap: theme.space[4],
	minWidth: 0,
})

export const titleRow = style({
	display: 'flex',
	flexWrap: 'wrap',
	alignItems: 'baseline',
	gap: `${theme.space[8]} ${theme.space[12]}`,
})

export const title = style({
	fontWeight: theme.typography.fontWeight.semibold,
	letterSpacing: theme.typography.letterSpacing.tighter,
	lineHeight: '32px',
})

export const titleSizes = styleVariants({
	md: { fontSize: theme.typography.fontSize[24] },
	// El nombre de la app en su encabezado.
	lg: { fontSize: theme.typography.fontSize[28] },
})

export const meta = style({
	color: theme.colors.text.tertiary,
})

export const subtitle = style({
	color: theme.colors.text.secondary,
})

export const actions = style({
	display: 'flex',
	flexWrap: 'wrap',
	alignItems: 'center',
	gap: theme.space[8],
})

export type PageTitleSize = keyof typeof titleSizes
