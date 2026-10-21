import { style, styleVariants } from '@vanilla-extract/css'
import { theme } from '@/shared/ui/themes/theme.css'

export const root = style({
	position: 'relative',
	minWidth: 0,
})

export const scroller = style({
	display: 'flex',
	flexDirection: 'column',
	overflowY: 'auto',
	overflowX: 'hidden',
	backgroundColor: theme.colors.background.page,
	fontFamily: theme.typography.fontFamily.mono,
	fontSize: theme.typography.fontSize[12],
	lineHeight: theme.typography.lineHeight.relaxed,
	overscrollBehavior: 'contain',
	// Mientras se sigue el final, el navegador no corre la vista por su cuenta cuando
	// cambia el alto de las líneas: lo maneja el componente (ver `anchored`).
	overflowAnchor: 'none',
	selectors: {
		'&:focus-visible': {
			outlineOffset: '-2px',
		},
	},
})

// Sin seguir (quien lee subió): el navegador mantiene quieto lo que se está leyendo
// aunque las líneas de arriba se pinten más altas que su alto estimado.
export const anchored = style({
	overflowAnchor: 'auto',
})

// Lo que se mide para volver a bajar: crece con las líneas, y llena la caja para que la
// lista se apile desde abajo (`margin-top: auto`) y el aviso vacío quede al centro.
export const content = style({
	display: 'flex',
	flexDirection: 'column',
	flex: '1 0 auto',
	minWidth: 0,
})

// `md`: el log de un deploy (380–560 px). `lg`: los logs en vivo (620 px, o menos en un
// teléfono).
export const sizes = styleVariants({
	md: { minHeight: '380px', maxHeight: '560px' },
	lg: { height: 'min(620px, 70vh)' },
})

export const jumpSlot = style({
	position: 'absolute',
	right: theme.space[16],
	bottom: theme.space[12],
})
