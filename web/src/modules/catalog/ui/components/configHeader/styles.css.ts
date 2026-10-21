import { style } from '@vanilla-extract/css'
import { theme } from '@/shared/ui/themes/theme.css'

export const details = style({
	display: 'flex',
	flexDirection: 'column',
	gap: theme.space[6],
	minWidth: 0,
})

export const subtitle = style({
	margin: 0,
	color: theme.colors.text.secondary,
})
