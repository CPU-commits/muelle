import { style } from '@vanilla-extract/css'
import { theme } from '@/shared/ui/themes/theme.css'

export const panel = style({
	display: 'flex',
	flexDirection: 'column',
	alignItems: 'center',
	gap: theme.space[8],
	textAlign: 'center',
	paddingBlock: theme.space[40],
	paddingInline: theme.space[24],
	border: `1px solid ${theme.colors.status.errorBorder}`,
	borderRadius: theme.radius.lg,
	backgroundColor: theme.colors.status.errorSurface,
})

export const title = style({
	display: 'inline-flex',
	alignItems: 'center',
	gap: theme.space[8],
	fontWeight: theme.typography.fontWeight.medium,
})

export const body = style({
	color: theme.colors.text.secondary,
	maxWidth: '52ch',
})

export const action = style({
	marginTop: theme.space[8],
})
