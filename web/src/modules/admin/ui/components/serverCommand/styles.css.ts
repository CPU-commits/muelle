import { style } from '@vanilla-extract/css'
import { theme } from '@/shared/ui/themes/theme.css'

// Un comando que se corre en el servidor, para copiarlo: qué hace, el comando y el botón.
export const block = style({
	display: 'flex',
	flexDirection: 'column',
	alignItems: 'flex-start',
	gap: theme.space[8],
	minWidth: 0,
})

export const label = style({
	margin: 0,
	fontSize: theme.typography.fontSize[13],
	color: theme.colors.text.secondary,
})

export const command = style({
	margin: 0,
	width: '100%',
	boxSizing: 'border-box',
	padding: `${theme.space[10]} ${theme.space[12]}`,
	border: `1px solid ${theme.colors.borders.subtle}`,
	borderRadius: theme.radius.md,
	backgroundColor: theme.colors.background.page,
	color: theme.colors.text.secondary,
	fontFamily: theme.typography.fontFamily.mono,
	fontSize: theme.typography.fontSize[12],
	lineHeight: 1.6,
	whiteSpace: 'pre-wrap',
	overflowWrap: 'anywhere',
})
