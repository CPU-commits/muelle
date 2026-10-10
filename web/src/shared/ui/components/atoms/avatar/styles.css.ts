import { style, styleVariants } from '@vanilla-extract/css'
import { theme } from '@/shared/ui/themes/theme.css'

export const avatar = style({
	display: 'inline-grid',
	placeItems: 'center',
	flexShrink: 0,
	borderRadius: theme.radius.full,
	border: `1px solid ${theme.colors.borders.control}`,
	backgroundColor: theme.colors.background.raised,
	color: theme.colors.text.primary,
	fontWeight: theme.typography.fontWeight.medium,
	lineHeight: 1,
	textTransform: 'uppercase',
	userSelect: 'none',
})

export const sizes = styleVariants({
	sm: { width: '28px', height: '28px', fontSize: theme.typography.fontSize[12] },
	md: { width: '32px', height: '32px', fontSize: theme.typography.fontSize[13] },
})

export type AvatarSize = keyof typeof sizes
