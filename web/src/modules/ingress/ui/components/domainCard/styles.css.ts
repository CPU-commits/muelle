import { style, styleVariants } from '@vanilla-extract/css'
import { theme } from '@/shared/ui/themes/theme.css'

export const card = style({
	border: `1px solid ${theme.colors.borders.subtle}`,
	borderRadius: theme.radius.lg,
	backgroundColor: theme.colors.background.surface,
	overflow: 'hidden',
	minWidth: 0,
})

export const main = style({
	display: 'flex',
	flexWrap: 'wrap',
	alignItems: 'center',
	justifyContent: 'space-between',
	gap: `${theme.space[12]} ${theme.space[16]}`,
	padding: `${theme.space[16]} ${theme.space[12]} ${theme.space[16]} ${theme.space[20]}`,
})

export const info = style({
	display: 'flex',
	flexDirection: 'column',
	gap: theme.space[6],
	minWidth: 0,
})

export const titleRow = style({
	display: 'flex',
	flexWrap: 'wrap',
	alignItems: 'center',
	gap: `${theme.space[6]} ${theme.space[12]}`,
	minWidth: 0,
})

export const title = style({
	fontSize: theme.typography.fontSize[15],
	fontWeight: theme.typography.fontWeight.semibold,
	overflowWrap: 'anywhere',
})

export const link = style({
	display: 'inline-flex',
	alignItems: 'center',
	gap: theme.space[6],
	textDecoration: 'none',
	color: theme.colors.text.primary,
	selectors: {
		'&:hover': { textDecoration: 'underline', textUnderlineOffset: '3px' },
	},
})

export const linkIcon = style({
	display: 'inline-flex',
	color: theme.colors.text.tertiary,
})

export const state = style({
	display: 'inline-flex',
	alignItems: 'center',
	gap: theme.space[6],
	fontSize: theme.typography.fontSize[13],
})

export const stateTones = styleVariants({
	ok: { color: theme.colors.text.secondary },
	warn: { color: theme.colors.status.warnText },
	bad: { color: theme.colors.status.errorText },
	idle: { color: theme.colors.text.secondary },
})

export const meta = style({
	display: 'flex',
	flexWrap: 'wrap',
	gap: `${theme.space[4]} ${theme.space[16]}`,
	fontSize: theme.typography.fontSize[13],
	color: theme.colors.text.tertiary,
})

export const actions = style({
	display: 'flex',
	flexWrap: 'wrap',
	alignItems: 'center',
	gap: theme.space[4],
})

export const proxied = style({
	display: 'inline-flex',
	alignItems: 'center',
	gap: theme.space[8],
	minHeight: theme.control.sm,
	padding: `0 ${theme.space[8]}`,
	fontSize: theme.typography.fontSize[13],
	color: theme.colors.text.secondary,
	cursor: 'pointer',
})

// El pie más oscuro: adoptar, el error con reintento o los registros a mano.
export const footer = style({
	display: 'flex',
	flexWrap: 'wrap',
	alignItems: 'center',
	justifyContent: 'space-between',
	gap: `${theme.space[12]} ${theme.space[24]}`,
	padding: `${theme.space[14]} ${theme.space[20]}`,
	borderTop: `1px solid ${theme.colors.borders.subtle}`,
	backgroundColor: theme.colors.background.sunken,
})

export const footerStack = style({
	flexDirection: 'column',
	alignItems: 'stretch',
	paddingBottom: theme.space[20],
})

export const footerText = style({
	flex: '1 1 420px',
	minWidth: 0,
	color: theme.colors.text.secondary,
	overflowWrap: 'anywhere',
})

// En el pie apilado (registros a mano) el texto no lleva `flex-basis`: sería su alto.
export const manualText = style({
	color: theme.colors.text.secondary,
})

export const errorText = style({
	color: theme.colors.status.errorText,
})

export const recordsBox = style({
	overflowX: 'auto',
	border: `1px solid ${theme.colors.borders.subtle}`,
	borderRadius: theme.radius.md,
})

export const copyCell = style({
	width: '44px',
	paddingTop: theme.space[4],
	paddingBottom: theme.space[4],
})
