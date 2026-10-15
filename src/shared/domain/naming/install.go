package naming

// LabelSwarmService la pone Swarm en cada contenedor de una tarea, con el
// nombre del servicio. muelle settings reset-panel-ips la lee en su propio
// contenedor para saber qué servicio reiniciar (contrato §5.1 y §5.4).
const LabelSwarmService = "com.docker.swarm.service.name"
