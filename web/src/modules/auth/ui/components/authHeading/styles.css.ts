import { style } from '@vanilla-extract/css'
import { theme } from '@/shared/ui/themes/theme.css'

export const heading = style({
	display: 'flex',
	flexDirection: 'column',
	gap: theme.space[8],
	textAlign: 'center',
})

export const title = style({
	fontSize: theme.typography.fontSize[24],
	lineHeight: '32px',
	fontWeight: theme.typography.fontWeight.semibold,
	letterSpacing: theme.typography.letterSpacing.tighter,
})

export const subtitle = style({
	color: theme.colors.text.secondary,
})
