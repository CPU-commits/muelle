import { style } from '@vanilla-extract/css'
import { theme } from '@/shared/ui/themes/theme.css'

export const frame = style({
	display: 'block',
	width: '100%',
	maxWidth: '200px',
	aspectRatio: '1 / 1',
	marginInline: 'auto',
	borderRadius: theme.radius.lg,
	backgroundColor: theme.colors.qr.background,
})

export const modules = style({
	fill: theme.colors.qr.foreground,
})
