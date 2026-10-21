import { style } from '@vanilla-extract/css'
import { theme } from '@/shared/ui/themes/theme.css'

export const panel = style({
	display: 'flex',
	flexDirection: 'column',
	alignItems: 'center',
	gap: theme.space[8],
	textAlign: 'center',
	paddingBlock: theme.space[48],
	paddingInline: theme.space[24],
	border: `1px dashed ${theme.colors.borders.control}`,
	borderRadius: theme.radius.lg,
	backgroundColor: theme.colors.background.surface,
})

export const icon = style({
	display: 'grid',
	placeItems: 'center',
	width: '36px',
	height: '36px',
	marginBottom: theme.space[4],
	borderRadius: theme.radius.lg,
	border: `1px solid ${theme.colors.borders.control}`,
	backgroundColor: theme.colors.background.raised,
	color: theme.colors.text.secondary,
})

export const title = style({
	fontSize: theme.typography.fontSize[15],
	fontWeight: theme.typography.fontWeight.semibold,
	letterSpacing: theme.typography.letterSpacing.snug,
})

export const body = style({
	color: theme.colors.text.secondary,
	maxWidth: '52ch',
})

export const action = style({
	display: 'flex',
	flexWrap: 'wrap',
	justifyContent: 'center',
	gap: theme.space[8],
	marginTop: theme.space[8],
})
