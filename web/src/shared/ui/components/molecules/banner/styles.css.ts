import { style, styleVariants } from '@vanilla-extract/css'
import { theme } from '@/shared/ui/themes/theme.css'

export const banner = style({
	display: 'flex',
	flexWrap: 'wrap',
	alignItems: 'center',
	gap: `${theme.space[8]} ${theme.space[12]}`,
	padding: `${theme.space[10]} ${theme.space[12]} ${theme.space[10]} ${theme.space[16]}`,
	border: '1px solid',
	borderRadius: theme.radius.lg,
})

export const tones = styleVariants({
	info: {
		borderColor: theme.colors.borders.control,
		backgroundColor: theme.colors.background.surface,
	},
	warn: {
		borderColor: theme.colors.status.warnBorder,
		backgroundColor: theme.colors.status.warnSurface,
	},
	error: {
		borderColor: theme.colors.status.errorBorder,
		backgroundColor: theme.colors.status.errorSurface,
	},
})

export const text = style({
	flex: '1 1 320px',
	minWidth: 0,
})

export const title = style({
	fontWeight: theme.typography.fontWeight.medium,
})

export const body = style({
	color: theme.colors.text.secondary,
})

export const action = style({
	display: 'flex',
	flexWrap: 'wrap',
	gap: theme.space[8],
})

export type BannerTone = keyof typeof tones
