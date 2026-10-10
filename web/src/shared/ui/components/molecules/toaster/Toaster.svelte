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
</script>

<div aria-live="polite" class={region}>
	{#each toasts.items as item (item.id)}
		<div class={toast} role={item.tone === 'error' ? 'alert' : 'status'}>
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
