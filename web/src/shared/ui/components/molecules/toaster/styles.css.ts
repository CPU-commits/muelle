import { style } from '@vanilla-extract/css'
import { toastIn } from '@/shared/ui/themes/animations.css'
import { theme } from '@/shared/ui/themes/theme.css'

export const region = style({
	position: 'fixed',
	right: theme.space[16],
	bottom: theme.space[16],
	left: theme.space[16],
	zIndex: theme.zIndex.toast,
	display: 'flex',
	flexDirection: 'column',
	alignItems: 'flex-end',
	gap: theme.space[8],
	pointerEvents: 'none',
})

export const toast = style({
	display: 'flex',
	alignItems: 'flex-start',
	gap: theme.space[10],
	width: '100%',
	maxWidth: '380px',
	padding: `${theme.space[12]} ${theme.space[8]} ${theme.space[12]} ${theme.space[14]}`,
	border: `1px solid ${theme.colors.borders.control}`,
	borderRadius: theme.radius.lg,
	backgroundColor: theme.colors.background.surface,
	boxShadow: theme.shadows.menu,
	pointerEvents: 'auto',
	animation: `${toastIn} 0.18s ease-out`,
})

export const dot = style({
	display: 'flex',
	paddingTop: '6px',
})

export const content = style({
	flex: 1,
	minWidth: 0,
	display: 'flex',
	flexDirection: 'column',
	gap: theme.space[2],
})

export const title = style({
	fontWeight: theme.typography.fontWeight.medium,
})

export const description = style({
	fontSize: theme.typography.fontSize[13],
	color: theme.colors.text.secondary,
	overflowWrap: 'anywhere',
})

export const close = style({
	display: 'grid',
	placeItems: 'center',
	width: theme.control.xs,
	height: theme.control.xs,
	marginBlock: `calc(${theme.space[4]} * -1)`,
	borderRadius: theme.radius.md,
	color: theme.colors.text.tertiary,
	selectors: {
		'&:hover': {
			backgroundColor: theme.colors.background.raised,
			color: theme.colors.text.primary,
		},
	},
})
