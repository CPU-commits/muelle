import { style } from '@vanilla-extract/css'
import { theme } from '@/shared/ui/themes/theme.css'

export const form = style({
	display: 'flex',
	flexDirection: 'column',
	gap: theme.space[24],
})

export const fields = style({
	display: 'flex',
	flexDirection: 'column',
	gap: theme.space[16],
})

export const codeFooter = style({
	display: 'flex',
	flexDirection: 'column',
	alignItems: 'center',
	gap: theme.space[12],
})

export const note = style({
	textAlign: 'center',
	fontSize: theme.typography.fontSize[13],
	color: theme.colors.text.tertiary,
})
