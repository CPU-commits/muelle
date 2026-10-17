<script lang="ts">
	import { CopyButton } from '@/shared/ui'
	import { block, command as commandClass, label as labelClass } from './styles.css'

	// Un comando para correr en el servidor (recuperar el acceso, actualizar
	// MUELLE_PANEL_ORIGIN): qué hace, el comando a la vista y el botón para copiarlo. Si el
	// navegador no deja copiar, CopyButton selecciona el texto del <pre>.
	type Props = { label: string; command: string; copyLabel: string; class?: string }

	let { label, command, copyLabel, class: className }: Props = $props()

	const commandId = $props.id()
</script>

<div class={[block, className]}>
	<p class={labelClass}>{label}</p>
	<pre id={commandId} class={commandClass}>{command}</pre>
	<CopyButton
		value={command}
		target={commandId}
		label={copyLabel}
		variant="secondary"
		size="sm"
	/>
</div>
