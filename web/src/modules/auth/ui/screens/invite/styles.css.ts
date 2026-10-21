import { style } from '@vanilla-extract/css'
import { theme } from '@/shared/ui/themes/theme.css'

export const screen = style({
	display: 'flex',
	flexDirection: 'column',
	gap: theme.space[24],
})
