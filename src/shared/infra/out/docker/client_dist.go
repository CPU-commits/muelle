package docker

import (
	"context"

	"github.com/moby/moby/api/types/swarm"
	"github.com/moby/moby/client"
)

var _ ServiceRestarter = (*Client)(nil)

// ForceUpdateService hace lo mismo que docker service update --force sin
// --with-registry-auth: registryAuthFrom=spec deja las credenciales que el
// servicio ya tenía (una imagen privada se sigue pudiendo bajar), y sin
// QueryRegistry no vuelve a resolver la imagen (el stack se desplegó con
// --resolve-image=never).
func (c *Client) ForceUpdateService(ctx context.Context, nameOrID string) error {
	res, err := c.api.ServiceInspect(ctx, nameOrID, client.ServiceInspectOptions{})
	if err != nil {
		return wrap("inspeccionando el servicio "+nameOrID, err)
	}
	spec := res.Service.Spec
	spec.TaskTemplate.ForceUpdate++
	_, err = c.api.ServiceUpdate(ctx, res.Service.ID, client.ServiceUpdateOptions{
		Version:          res.Service.Version,
		Spec:             spec,
		RegistryAuthFrom: swarm.RegistryAuthFromSpec,
	})
	return wrap("reiniciando el servicio "+nameOrID, err)
}
