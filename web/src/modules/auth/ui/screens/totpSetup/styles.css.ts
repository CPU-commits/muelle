import { style } from '@vanilla-extract/css'
import { theme } from '@/shared/ui/themes/theme.css'

export const screen = style({
	display: 'flex',
	flexDirection: 'column',
	gap: theme.space[24],
})

export const qrBlock = style({
	display: 'flex',
	flexDirection: 'column',
	alignItems: 'center',
	gap: theme.space[16],
	padding: theme.space[20],
	border: `1px solid ${theme.colors.borders.subtle}`,
	borderRadius: theme.radius.lg,
	backgroundColor: theme.colors.background.surface,
})

export const secret = style({
	display: 'flex',
	flexDirection: 'column',
	alignItems: 'center',
	gap: theme.space[4],
	textAlign: 'center',
})

export const secretLabel = style({
	fontSize: theme.typography.fontSize[13],
	color: theme.colors.text.tertiary,
})

export const secretRow = style({
	display: 'flex',
	alignItems: 'center',
	gap: theme.space[4],
	overflowWrap: 'anywhere',
})

export const form = style({
	display: 'flex',
	flexDirection: 'column',
	gap: theme.space[24],
})

export const footer = style({
	display: 'flex',
	justifyContent: 'center',
})
