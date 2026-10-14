<script lang="ts">
	import { onDestroy } from 'svelte'
	import type { Attachment } from 'svelte/attachments'
	import { m } from '@/shared/ui/i18n/paraglide/messages'
	import { toast } from '@/shared/ui/stores'
	import { srOnly } from '@/shared/ui/themes/utilities.css'
	import Button from '../../atoms/button/Button.svelte'
	import type { ButtonSize, ButtonVariant } from '../../atoms/button/styles.css'
	import Icon from '../../atoms/icon/Icon.svelte'
	import { selectText, writeToClipboard } from './clipboard'

	// Copia `value` al portapapeles y muestra "Copiado" un momento. Para el token recién
	// creado, la URL de una invitación, un digest. Si no se puede copiar (panel en HTTP,
	// permiso denegado), lo dice y selecciona el texto de `target` para copiarlo a mano.
	type Props = {
		value: string
		/** Texto del botón ("Copiar"), o su `aria-label` si es `iconOnly` ("Copiar digest"). */
		label?: string
		/** El id del elemento que muestra el valor (un input de solo lectura, un <code>). */
		target?: string
		iconOnly?: boolean
		variant?: ButtonVariant
		size?: ButtonSize
	}

	let {
		value,
		label,
		target,
		iconOnly = false,
		variant = 'secondary',
		size = 'md',
	}: Props = $props()

	const FEEDBACK_MS = 1600
	let result = $state<'idle' | 'copied' | 'failed'>('idle')
	// Para saber si el botón está adentro de un modal (ver `writeToClipboard`).
	let liveRegion: HTMLSpanElement | undefined
	const rememberLiveRegion: Attachment<HTMLSpanElement> = (element) => {
		liveRegion = element
		return () => {
			liveRegion = undefined
		}
	}
	let timer: ReturnType<typeof setTimeout> | undefined
	// Un timer pendiente (1,6 s; 3,2 s si falló) retiene este componente, y con él el
	// <dialog> que lo contiene y el token que mostraba: se corta al desmontar.
	onDestroy(() => clearTimeout(timer))

	function show(next: 'copied' | 'failed'): void {
		result = next
		clearTimeout(timer)
		timer = setTimeout(
			() => (result = 'idle'),
			next === 'failed' ? FEEDBACK_MS * 2 : FEEDBACK_MS,
		)
	}

	async function copy(): Promise<void> {
		if (await writeToClipboard(value, liveRegion)) {
			show('copied')
			return
		}
		show('failed')
		toast.error(m.common_copy_failed())
		if (target) selectText(target)
	}

	const text = $derived(
		result === 'copied'
			? m.common_copied()
			: result === 'failed'
				? m.common_copy_failed_short()
				: (label ?? m.common_copy()),
	)
</script>

<Button
	{variant}
	{size}
	{iconOnly}
	aria-label={iconOnly ? text : undefined}
	onclick={() => void copy()}
>
	<Icon name={result === 'copied' ? 'check' : result === 'failed' ? 'alert' : 'copy'} size={14} />
	{#if !iconOnly}{text}{/if}
</Button>
<span {@attach rememberLiveRegion} class={srOnly} aria-live="polite"
	>{result === 'copied' ? m.common_copied() : ''}</span
>
