import { globalStyle, style, styleVariants } from '@vanilla-extract/css'
import { backdropIn, modalIn } from '@/shared/ui/themes/animations.css'
import { theme } from '@/shared/ui/themes/theme.css'

// <dialog> nativo con showModal(): el navegador pone el fondo inerte, atrapa el foco,
// cierra con Escape y devuelve el foco al botón que lo abrió.
export const dialog = style({
	width: 'calc(100% - 32px)',
	marginTop: 'min(120px, 12vh)',
	marginInline: 'auto',
	padding: 0,
	border: `1px solid ${theme.colors.borders.control}`,
	borderRadius: theme.radius.xl,
	backgroundColor: theme.colors.background.surface,
	color: theme.colors.text.primary,
	boxShadow: theme.shadows.modal,
	overflow: 'hidden',
	animation: `${modalIn} 0.18s ease-out`,
	selectors: {
		'&:focus-visible': { outline: 'none' },
	},
	'@media': {
		'(prefers-reduced-motion: reduce)': { animation: 'none' },
	},
})

export const sizes = styleVariants({
	sm: { maxWidth: '400px' },
	md: { maxWidth: '480px' },
	lg: { maxWidth: '640px' },
})

globalStyle(`${dialog}::backdrop`, {
	backgroundColor: theme.colors.overlay,
	animation: `${backdropIn} 0.15s ease-out`,
})

export const body = style({
	display: 'flex',
	flexDirection: 'column',
	gap: theme.space[20],
	padding: theme.space[24],
})

export const heading = style({
	display: 'flex',
	flexDirection: 'column',
	gap: theme.space[6],
})

export const title = style({
	fontSize: theme.typography.fontSize[18],
	fontWeight: theme.typography.fontWeight.semibold,
	letterSpacing: theme.typography.letterSpacing.tight,
	lineHeight: theme.typography.lineHeight.snug,
})

export const description = style({
	color: theme.colors.text.secondary,
})

export const footer = style({
	display: 'flex',
	flexWrap: 'wrap',
	justifyContent: 'flex-end',
	gap: theme.space[8],
	padding: `${theme.space[12]} ${theme.space[24]}`,
	borderTop: `1px solid ${theme.colors.borders.subtle}`,
	backgroundColor: theme.colors.background.sunken,
})

export type ModalSize = keyof typeof sizes
