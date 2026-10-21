<script lang="ts">
	import { m } from '@/shared/ui/i18n/paraglide/messages'
	import { toasts, type ToastTone } from '@/shared/ui/stores'
	import Icon from '../../atoms/icon/Icon.svelte'
	import StatusDot from '../../atoms/statusDot/StatusDot.svelte'
	import type { StatusTone } from '../../atoms/statusDot/styles.css'
	import {
		close,
		content,
		description as descriptionClass,
		dot,
		region,
		title as titleClass,
		toast,
	} from './styles.css'

	const DOT_BY_TONE: Record<ToastTone, StatusTone> = { success: 'ok', error: 'bad', info: 'idle' }

	// Si el foco sale de un aviso hacia otro aviso, la cuenta sigue en pausa.
	function onFocusOut(event: FocusEvent & { currentTarget: HTMLElement }): void {
		const next = event.relatedTarget
		if (!(next instanceof Node) || !event.currentTarget.parentElement?.contains(next)) {
			toasts.resume()
		}
	}
</script>

<!-- Una sola región viva (polite): cada aviso nuevo se lee una vez, sin roles anidados
     que lo anuncien de nuevo. -->
<div aria-live="polite" aria-relevant="additions" class={region}>
	{#each toasts.items as item (item.id)}
		<!-- Pausar con el puntero encima es un extra del mouse; con teclado lo hacen
		     focusin/focusout. Un role acá volvería a anidar roles en la región viva. -->
		<!-- svelte-ignore a11y_no_static_element_interactions -->
		<div
			class={toast}
			data-toast={item.tone}
			onmouseenter={() => toasts.pause()}
			onmouseleave={() => toasts.resume()}
			onfocusin={() => toasts.pause()}
			onfocusout={onFocusOut}
		>
			<span class={dot}><StatusDot status={DOT_BY_TONE[item.tone]} /></span>
			<div class={content}>
				<p class={titleClass}>{item.title}</p>
				{#if item.description}
					<p class={descriptionClass}>{item.description}</p>
				{/if}
			</div>
			<button
				type="button"
				aria-label={m.common_close()}
				class={close}
				onclick={() => toasts.dismiss(item.id)}
			>
				<Icon name="x" size={14} />
			</button>
		</div>
	{/each}
</div>
