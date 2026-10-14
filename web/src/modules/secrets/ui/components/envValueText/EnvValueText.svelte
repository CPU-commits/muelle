<script lang="ts">
	import { m } from '@/shared/ui/i18n/paraglide/messages'
	import { Icon, srOnly } from '@/shared/ui'
	import type { EnvVar } from '../../../domain'
	import {
		empty,
		lock,
		mask,
		masked,
		secretLabel,
		struck as struckClass,
		text,
		tones,
		type ValueTone,
	} from './styles.css'

	// El valor de una variable en una fila: el texto (recortado, entero en el `title`), o
	// el candado y los puntos si es secreta. Un valor secreto nunca llega al panel.
	type Props = {
		envVar: EnvVar
		/** `secondary` en las de la app; `tertiary` en las heredadas del stage. */
		tone?: ValueTone
		/** Tachado: una heredada que la app reemplaza. */
		struck?: boolean
		/** Muestra "secreta" después de los puntos (las heredadas no tienen candado aparte). */
		labelSecret?: boolean
	}

	let { envVar, tone = 'secondary', struck = false, labelSecret = false }: Props = $props()
</script>

{#if envVar.secret}
	<span class={[masked, tones[tone], struck && struckClass]}>
		<span class={lock}><Icon name="lock" size={13} /></span>
		<span class={mask} aria-hidden="true">••••••••••••</span>
		{#if labelSecret}
			<span class={secretLabel}>{m.secrets_inherited_secret()}</span>
		{:else}
			<span class={srOnly}>{m.secrets_secret_masked()}</span>
		{/if}
	</span>
{:else if envVar.value === ''}
	<span class={[empty, struck && struckClass]}>{m.secrets_value_empty()}</span>
{:else}
	<span class={[text, tones[tone], struck && struckClass]} title={envVar.value}
		>{envVar.value}</span
	>
{/if}
