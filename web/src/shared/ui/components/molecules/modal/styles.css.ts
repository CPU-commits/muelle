import { globalStyle, style, styleVariants } from '@vanilla-extract/css'
import { backdropIn, modalIn } from '@/shared/ui/themes/animations.css'
import { theme } from '@/shared/ui/themes/theme.css'

// El margen de arriba y el mínimo de abajo: el diálogo nunca pasa del alto de la pantalla.
const TOP = 'min(120px, 12vh)'
const BOTTOM = '16px'

// <dialog> nativo con showModal(): el navegador pone el fondo inerte, atrapa el foco,
// cierra con Escape y devuelve el foco al botón que lo abrió.
//
// En una pantalla baja (un teléfono, 360×640) con un formulario largo, el pie (los
// botones) tiene que seguir a la vista: el diálogo se limita al alto disponible y lo que
// scrollea es el cuerpo.
export const dialog = style({
	width: 'calc(100% - 32px)',
	maxHeight: [`calc(100vh - ${TOP} - ${BOTTOM})`, `calc(100dvh - ${TOP} - ${BOTTOM})`],
	marginTop: TOP,
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
		// Solo abierto: un `display` en el <dialog> cerrado le ganaría al `display: none`
		// del navegador.
		'&[open]': { display: 'flex', flexDirection: 'column' },
		// Reabierto a la fuerza (`dismissible={false}`): no se repite la entrada.
		'&[data-reopened]': { animation: 'none' },
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
	flex: '1 1 auto',
	minHeight: 0,
	overflowY: 'auto',
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
	flexShrink: 0,
})

export type ModalSize = keyof typeof sizes
