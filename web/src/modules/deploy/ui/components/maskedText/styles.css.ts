import { style } from '@vanilla-extract/css'
import { theme } from '@/shared/ui/themes/theme.css'

// Un `***` que puso el backend en lugar de un secreto: se distingue del texto real.
export const masked = style({
	padding: `0 ${theme.space[2]}`,
	borderRadius: theme.radius.xs,
	backgroundColor: theme.colors.background.raised,
	color: theme.colors.status.warnText,
	cursor: 'help',
})
