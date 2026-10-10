import { style } from '@vanilla-extract/css'
import { theme } from '@/shared/ui/themes/theme.css'

export const pagination = style({
	display: 'flex',
	alignItems: 'center',
	justifyContent: 'space-between',
	gap: theme.space[12],
})

export const label = style({
	fontSize: theme.typography.fontSize[13],
	color: theme.colors.text.tertiary,
})
