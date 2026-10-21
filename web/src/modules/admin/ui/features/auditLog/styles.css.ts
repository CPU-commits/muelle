import { style } from '@vanilla-extract/css'
import { theme } from '@/shared/ui/themes/theme.css'

export const section = style({
	display: 'flex',
	flexDirection: 'column',
	gap: theme.space[10],
	minWidth: 0,
})

export const when = style({
	color: theme.colors.text.secondary,
	whiteSpace: 'nowrap',
})

export const target = style({
	color: theme.colors.text.secondary,
	overflowWrap: 'anywhere',
})

export const moreError = style({
	textAlign: 'center',
	fontSize: theme.typography.fontSize[13],
	color: theme.colors.status.errorText,
})
