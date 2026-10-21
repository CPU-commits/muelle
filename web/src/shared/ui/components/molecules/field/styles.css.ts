import { style } from '@vanilla-extract/css'
import { theme } from '@/shared/ui/themes/theme.css'

export const field = style({
	display: 'flex',
	flexDirection: 'column',
	gap: theme.space[8],
	minWidth: 0,
})

export const label = style({
	fontSize: theme.typography.fontSize[13],
	fontWeight: theme.typography.fontWeight.medium,
	color: theme.colors.text.secondary,
})

export const optional = style({
	fontWeight: theme.typography.fontWeight.regular,
	color: theme.colors.text.tertiary,
})

export const message = style({
	fontSize: theme.typography.fontSize[13],
	color: theme.colors.text.tertiary,
	marginTop: `calc(${theme.space[2]} * -1)`,
})

export const messageError = style({
	color: theme.colors.status.errorText,
})
