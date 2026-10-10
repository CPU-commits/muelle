import { style, styleVariants } from '@vanilla-extract/css'
import { theme } from '@/shared/ui/themes/theme.css'

export const card = style({
	border: '1px solid',
	borderRadius: theme.radius.lg,
	backgroundColor: theme.colors.background.surface,
	overflow: 'hidden',
	minWidth: 0,
})

export const tones = styleVariants({
	default: { borderColor: theme.colors.borders.subtle },
	// La tarjeta de "Eliminar app": borde y pie rojizos.
	danger: { borderColor: theme.colors.status.errorBorder },
	// La tarjeta del token recién creado: un borde más claro que llama la atención.
	highlight: { borderColor: theme.colors.borders.strong },
})

export const body = style({
	display: 'flex',
	flexDirection: 'column',
	gap: theme.space[16],
	padding: `${theme.space[20]} ${theme.space[24]}`,
})

export const bandHeader = style({
	display: 'flex',
	flexWrap: 'wrap',
	alignItems: 'center',
	justifyContent: 'space-between',
	gap: `${theme.space[8]} ${theme.space[16]}`,
	padding: `${theme.space[14]} ${theme.space[20]}`,
	borderBottom: `1px solid ${theme.colors.borders.subtle}`,
})

export const bandMeta = style({
	display: 'flex',
	flexWrap: 'wrap',
	alignItems: 'baseline',
	gap: `${theme.space[4]} ${theme.space[12]}`,
	minWidth: 0,
	fontSize: theme.typography.fontSize[13],
	color: theme.colors.text.tertiary,
})

export const bandTitle = style({
	fontSize: theme.typography.fontSize[14],
	fontWeight: theme.typography.fontWeight.semibold,
	color: theme.colors.text.primary,
})

export const bandAside = style({
	display: 'flex',
	alignItems: 'center',
	gap: theme.space[8],
	fontSize: theme.typography.fontSize[12],
	color: theme.colors.text.tertiary,
})

export const heading = style({
	display: 'flex',
	flexDirection: 'column',
	gap: theme.space[4],
})

export const title = style({
	fontSize: theme.typography.fontSize[16],
	fontWeight: theme.typography.fontWeight.semibold,
	letterSpacing: theme.typography.letterSpacing.snug,
})

export const description = style({
	color: theme.colors.text.secondary,
	maxWidth: '68ch',
})

export const footer = style({
	display: 'flex',
	flexWrap: 'wrap',
	alignItems: 'center',
	justifyContent: 'space-between',
	gap: `${theme.space[8]} ${theme.space[16]}`,
	padding: `${theme.space[12]} ${theme.space[24]}`,
	borderTop: `1px solid ${theme.colors.borders.subtle}`,
	backgroundColor: theme.colors.background.sunken,
})

export const footerDanger = style({
	borderTopColor: theme.colors.status.errorDivider,
	backgroundColor: theme.colors.status.errorSunken,
})

export const footerNote = style({
	fontSize: theme.typography.fontSize[13],
	color: theme.colors.text.tertiary,
})

export const footerActions = style({
	display: 'flex',
	flexWrap: 'wrap',
	gap: theme.space[8],
	marginLeft: 'auto',
})

export type CardTone = keyof typeof tones
