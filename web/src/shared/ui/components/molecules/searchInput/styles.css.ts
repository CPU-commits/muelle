import { style } from '@vanilla-extract/css'
import { theme } from '@/shared/ui/themes/theme.css'

export const root = style({
	position: 'relative',
	display: 'flex',
	alignItems: 'center',
	minWidth: 0,
})

export const icon = style({
	position: 'absolute',
	left: theme.space[10],
	display: 'flex',
	color: theme.colors.text.tertiary,
	pointerEvents: 'none',
})

export const input = style({
	paddingLeft: theme.space[32],
	selectors: {
		'&::-webkit-search-cancel-button': { cursor: 'pointer' },
	},
})
