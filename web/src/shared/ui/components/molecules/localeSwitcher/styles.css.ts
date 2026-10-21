import { style } from '@vanilla-extract/css'
import { theme } from '@/shared/ui/themes/theme.css'

export const root = style({
	display: 'inline-flex',
	alignItems: 'center',
	gap: theme.space[8],
})

export const select = style({
	height: theme.control.sm,
	paddingInline: theme.space[8],
	backgroundColor: theme.colors.background.page,
	color: theme.colors.text.secondary,
	border: `1px solid ${theme.colors.borders.control}`,
	borderRadius: theme.radius.md,
	fontSize: theme.typography.fontSize[13],
	fontFamily: 'inherit',
	cursor: 'pointer',
})
