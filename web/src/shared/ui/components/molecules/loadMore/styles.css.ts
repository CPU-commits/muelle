import { style } from '@vanilla-extract/css'
import { theme } from '@/shared/ui/themes/theme.css'

export const row = style({
	display: 'flex',
	justifyContent: 'center',
	paddingBlock: theme.space[8],
})

export const done = style({
	fontSize: theme.typography.fontSize[13],
	color: theme.colors.text.tertiary,
})
