package dockertest

import "github.com/CPU-commits/muelle/src/shared/infra/out/docker"

// PutContainer crea o reemplaza, por nombre, un contenedor como los que crea
// Swarm para las tareas de un servicio (Task.ContainerID): sin
// CreateContainer, que exige la imagen en el nodo. InspectContainer lo
// encuentra por nombre o por id, con su Health y Running, y ContainerStats
// también (con SetStats por su nombre). Sin ID, se le asigna uno; sin
// nombre, se usa el ID. Lo usan los tests del estado de las apps (Fase 2).
func (f *Fake) PutContainer(c docker.Container) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if c.ID == "" {
		c.ID = f.id("ctr")
	}
	if c.Name == "" {
		c.Name = c.ID
	}
	stored := copyContainer(&c)
	f.containers[c.Name] = &stored
}
