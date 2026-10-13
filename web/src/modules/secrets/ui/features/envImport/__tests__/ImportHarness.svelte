<script lang="ts">
	import { setQueryClientContext } from '@tanstack/svelte-query'
	import { createQueryClient } from '@/shared/data/query'
	import { untrack } from 'svelte'
	import type { EnvOwner } from '../../../../domain'
	import { EnvEditorViewModel } from '../../envEditor'
	import { EnvImportViewModel } from '..'

	// Monta el editor del dueño y el de "Importar .env" encima, como en las pantallas. No
	// renderiza nada: el test habla con los viewModels.
	const props: {
		owner: EnvOwner
		onReady: (vms: { editor: EnvEditorViewModel; importer: EnvImportViewModel }) => void
	} = $props()

	untrack(() => {
		// Las mismas opciones que la app, sin reintentos.
		setQueryClientContext(createQueryClient({ retry: false }))
		const editor = new EnvEditorViewModel(() => props.owner)
		props.onReady({ editor, importer: new EnvImportViewModel(editor) })
	})
</script>
