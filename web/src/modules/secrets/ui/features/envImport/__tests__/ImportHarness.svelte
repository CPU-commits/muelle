<script lang="ts">
	import { type QueryClient, setQueryClientContext } from '@tanstack/svelte-query'
	import { untrack } from 'svelte'
	import type { EnvOwner } from '../../../../domain'
	import { EnvEditorViewModel } from '../../envEditor'
	import { EnvImportViewModel } from '..'

	// Monta el editor del dueño y el de "Importar .env" encima, como en las pantallas. No
	// renderiza nada: el test habla con los viewModels.
	const props: {
		client: QueryClient
		owner: EnvOwner
		onReady: (vms: { editor: EnvEditorViewModel; importer: EnvImportViewModel }) => void
	} = $props()

	untrack(() => {
		setQueryClientContext(props.client)
		const editor = new EnvEditorViewModel(() => props.owner)
		props.onReady({ editor, importer: new EnvImportViewModel(editor) })
	})
</script>
