import { style } from '@vanilla-extract/css'
import { theme } from '@/shared/ui/themes/theme.css'

export const nav = style({
	minWidth: 0,
	marginLeft: `calc(${theme.space[6]} * -1)`,
})

export const list = style({
	listStyle: 'none',
	margin: 0,
	padding: 0,
	display: 'flex',
	flexWrap: 'wrap',
	alignItems: 'center',
	gap: theme.space[2],
	minWidth: 0,
})

export const item = style({
	display: 'flex',
	alignItems: 'center',
	gap: theme.space[2],
	minWidth: 0,
})

export const separator = style({
	paddingInline: theme.space[2],
	color: theme.colors.text.separator,
	fontSize: theme.typography.fontSize[20],
	fontWeight: '300',
	userSelect: 'none',
})

export const home = style({
	display: 'grid',
	placeItems: 'center',
	width: theme.control.md,
	height: theme.control.md,
	borderRadius: theme.radius.md,
	color: theme.colors.text.primary,
	selectors: {
		'&:hover': { backgroundColor: theme.colors.background.raised },
	},
})

export const segment = style({
	display: 'inline-flex',
	alignItems: 'center',
	gap: theme.space[8],
	height: theme.control.md,
	paddingInline: theme.space[8],
	borderRadius: theme.radius.md,
	color: theme.colors.text.primary,
	fontWeight: theme.typography.fontWeight.medium,
	whiteSpace: 'nowrap',
})

export const segmentLink = style({
	selectors: {
		'&:hover': { backgroundColor: theme.colors.background.raised },
	},
})

// El cuadradito con la inicial del proyecto.
export const initial = style({
	display: 'grid',
	placeItems: 'center',
	width: '20px',
	height: '20px',
	borderRadius: theme.radius.sm,
	border: `1px solid ${theme.colors.borders.control}`,
	backgroundColor: theme.colors.background.raised,
	color: theme.colors.text.secondary,
	fontFamily: theme.typography.fontFamily.mono,
	fontSize: theme.typography.fontSize[11],
	fontWeight: theme.typography.fontWeight.regular,
})

// El selector ("auditfy ⇅"): la misma forma del segmento, como botón.
export const selector = style({
	border: 'none',
	background: 'transparent',
	cursor: 'pointer',
	selectors: {
		'&[aria-expanded="true"]': { backgroundColor: theme.colors.background.raised },
	},
})

export const chevron = style({
	display: 'flex',
	color: theme.colors.text.tertiary,
})
