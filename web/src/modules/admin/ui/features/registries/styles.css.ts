import { style } from '@vanilla-extract/css'
import { theme } from '@/shared/ui/themes/theme.css'

export const section = style({
	display: 'flex',
	flexDirection: 'column',
	gap: theme.space[10],
	minWidth: 0,
})

export const hostCell = style({
	flex: '1 1 180px',
	minWidth: 0,
	fontWeight: theme.typography.fontWeight.medium,
	overflowWrap: 'anywhere',
})

export const userCell = style({
	flex: '1 1 160px',
	minWidth: 0,
	color: theme.colors.text.secondary,
	overflowWrap: 'anywhere',
})

export const dateCell = style({
	flex: '0 0 auto',
	fontSize: theme.typography.fontSize[13],
	color: theme.colors.text.tertiary,
	whiteSpace: 'nowrap',
})

export const form = style({
	display: 'flex',
	flexDirection: 'column',
	gap: theme.space[16],
})
