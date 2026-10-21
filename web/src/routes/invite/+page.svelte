<script lang="ts">
	import { InviteScreen } from '@/modules/auth'
</script>

<!-- A donde queda la URL después de leer el token de /invite/<token> (se saca para que
     no quede en el historial). Si se recarga, la pantalla dice que falta el link. -->
<InviteScreen token={null} />
