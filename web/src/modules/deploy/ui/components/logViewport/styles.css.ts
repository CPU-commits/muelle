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
	// El navegador no corre la vista por su cuenta cuando cambia el alto de las líneas
	// (el seguimiento del final lo maneja el componente).
	overflowAnchor: 'none',
	selectors: {
		'&:focus-visible': {
			outlineOffset: '-2px',
		},
	},
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
