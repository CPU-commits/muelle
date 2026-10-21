import { keyframes } from '@vanilla-extract/css'
import { theme } from './theme.css'

export const spin = keyframes({
	to: { transform: 'rotate(360deg)' },
})

export const modalIn = keyframes({
	from: { opacity: 0, transform: 'translateY(8px) scale(0.98)' },
	to: { opacity: 1, transform: 'translateY(0) scale(1)' },
})

export const backdropIn = keyframes({
	from: { opacity: 0 },
	to: { opacity: 1 },
})

export const toastIn = keyframes({
	from: { opacity: 0, transform: 'translateY(8px)' },
	to: { opacity: 1, transform: 'translateY(0)' },
})

export const menuIn = keyframes({
	from: { opacity: 0, transform: 'translateY(-4px)' },
	to: { opacity: 1, transform: 'translateY(0)' },
})

// El punto "en vivo" del Resumen: un halo verde que se expande y se apaga.
export const pulse = keyframes({
	'0%': { boxShadow: `0 0 0 0 ${theme.colors.status.okPulse}` },
	'70%': { boxShadow: `0 0 0 6px ${theme.colors.status.okPulseEnd}` },
	'100%': { boxShadow: `0 0 0 0 ${theme.colors.status.okPulseEnd}` },
})
