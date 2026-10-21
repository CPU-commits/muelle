import { style } from '@vanilla-extract/css'

// Con una clase y no con un style="" en línea: la CSP del panel no permite estilos en línea.
export const icon = style({
	flexShrink: 0,
})
