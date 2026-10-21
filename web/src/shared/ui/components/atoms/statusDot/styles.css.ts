import { style, styleVariants } from '@vanilla-extract/css'
import { pulse as pulseKeyframes } from '@/shared/ui/themes/animations.css'
import { theme } from '@/shared/ui/themes/theme.css'

export const dot = style({
	display: 'inline-block',
	flexShrink: 0,
	borderRadius: theme.radius.full,
})

export const sizes = styleVariants({
	6: { width: '6px', height: '6px' },
	8: { width: '8px', height: '8px' },
})

// `idle` es un anillo sin relleno (una app sin desplegar), el resto un punto lleno.
export const statuses = styleVariants({
	ok: { backgroundColor: theme.colors.status.ok },
	warn: { backgroundColor: theme.colors.status.warn },
	bad: { backgroundColor: theme.colors.status.error },
	idle: {
		backgroundColor: 'transparent',
		boxShadow: `inset 0 0 0 1.5px ${theme.colors.status.idle}`,
	},
})

export const pulse = style({
	animation: `${pulseKeyframes} 2s infinite`,
	'@media': {
		'(prefers-reduced-motion: reduce)': { animation: 'none' },
	},
})

export type StatusTone = keyof typeof statuses
export type StatusDotSize = keyof typeof sizes
