<script lang="ts">
	import { m } from '@/shared/ui/i18n/paraglide/messages'
	import { srOnly } from '@/shared/ui/themes/utilities.css'
	import Button from '../../atoms/button/Button.svelte'
	import type { ButtonSize, ButtonVariant } from '../../atoms/button/styles.css'
	import Icon from '../../atoms/icon/Icon.svelte'

	// Copia `value` al portapapeles y muestra "Copiado" un momento. Para el token recién
	// creado, la URL de una invitación, un digest.
	type Props = {
		value: string
		/** Texto del botón ("Copiar"), o su `aria-label` si es `iconOnly` ("Copiar digest"). */
		label?: string
		iconOnly?: boolean
		variant?: ButtonVariant
		size?: ButtonSize
	}

	let { value, label, iconOnly = false, variant = 'secondary', size = 'md' }: Props = $props()

	const COPIED_MS = 1600
	let copied = $state(false)
	let timer: ReturnType<typeof setTimeout> | undefined

	async function copy(): Promise<void> {
		try {
			await navigator.clipboard.writeText(value)
		} catch {
			return
		}
		copied = true
		clearTimeout(timer)
		timer = setTimeout(() => (copied = false), COPIED_MS)
	}

	const text = $derived(copied ? m.common_copied() : (label ?? m.common_copy()))
</script>

<Button
	{variant}
	{size}
	{iconOnly}
	aria-label={iconOnly ? text : undefined}
	onclick={() => void copy()}
>
	<Icon name={copied ? 'check' : 'copy'} size={14} />
	{#if !iconOnly}{text}{/if}
</Button>
<span class={srOnly} aria-live="polite">{copied ? m.common_copied() : ''}</span>
