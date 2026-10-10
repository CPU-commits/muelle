import { style, styleVariants } from '@vanilla-extract/css'
import { theme } from '@/shared/ui/themes/theme.css'

// Las pantallas de acceso (`Login.dc.html`): logo arriba a la izquierda, el host a la
// derecha y un formulario angosto centrado.
export const page = style({
	minHeight: '100vh',
	display: 'flex',
	flexDirection: 'column',
	backgroundColor: theme.colors.background.page,
})

export const header = style({
	display: 'flex',
	flexWrap: 'wrap',
	alignItems: 'center',
	justifyContent: 'space-between',
	gap: theme.space[16],
	padding: `${theme.space[20]} ${theme.layout.gutter}`,
})

export const brand = style({
	display: 'inline-flex',
	alignItems: 'center',
	gap: theme.space[10],
	color: theme.colors.text.primary,
})

export const brandName = style({
	fontSize: theme.typography.fontSize[15],
	fontWeight: theme.typography.fontWeight.semibold,
	letterSpacing: theme.typography.letterSpacing.tight,
})

export const main = style({
	flex: '1 1 auto',
	display: 'flex',
	alignItems: 'center',
	justifyContent: 'center',
	padding: `${theme.space[24]} ${theme.space[16]} ${theme.space[80]}`,
})

export const content = style({
	width: '100%',
})

export const widths = styleVariants({
	sm: { maxWidth: theme.layout.authWidth },
	md: { maxWidth: '440px' },
})

export type AuthWidth = keyof typeof widths
