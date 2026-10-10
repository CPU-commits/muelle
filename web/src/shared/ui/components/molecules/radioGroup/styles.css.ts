import { style, styleVariants } from '@vanilla-extract/css'
import { theme } from '@/shared/ui/themes/theme.css'

export const group = style({
	display: 'flex',
	flexWrap: 'wrap',
	gap: `${theme.space[8]} ${theme.space[24]}`,
})

export const directions = styleVariants({
	row: { flexDirection: 'row' },
	column: { flexDirection: 'column', gap: theme.space[8] },
})

export const option = style({
	display: 'inline-flex',
	alignItems: 'center',
	gap: theme.space[8],
	fontSize: theme.typography.fontSize[13],
	color: theme.colors.text.secondary,
	cursor: 'pointer',
	selectors: {
		'&:has(input:disabled)': { opacity: 0.5, cursor: 'not-allowed' },
	},
})

export const input = style({
	margin: 0,
	width: '16px',
	height: '16px',
	accentColor: theme.colors.text.primary,
	cursor: 'inherit',
})
