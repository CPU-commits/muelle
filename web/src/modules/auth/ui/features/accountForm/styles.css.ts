import { style } from '@vanilla-extract/css'
import { theme } from '@/shared/ui/themes/theme.css'

export const fields = style({
	display: 'flex',
	flexDirection: 'column',
	gap: theme.space[16],
})

export const form = style({
	display: 'flex',
	flexDirection: 'column',
	gap: theme.space[24],
})
