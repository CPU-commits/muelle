import { style } from '@vanilla-extract/css'
import { theme } from '@/shared/ui/themes/theme.css'

export const row = style({
	display: 'flex',
	alignItems: 'center',
	justifyContent: 'center',
	gap: theme.space[8],
	paddingBlock: theme.space[48],
	color: theme.colors.text.tertiary,
})
