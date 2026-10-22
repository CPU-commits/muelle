package naming

// InitRegistrySecret es el secreto Swarm opcional con la credencial con que
// install bajó las imágenes de muelle, si vinieron de un registry privado: un JSON
// {"host", "username", "password"}, montado en
// /run/secrets/muelle_init_registry (MUELLE_INIT_REGISTRY_FILE). deploy lo
// guarda como un registry en el primer arranque de la base (contrato §2.10 y
// §5.2).
const InitRegistrySecret = "muelle_init_registry"
