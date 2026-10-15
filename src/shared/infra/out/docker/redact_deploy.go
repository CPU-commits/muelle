package docker

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
)

// Los tipos que llevan secretos se imprimen sin ellos (contrato §2.10):
// RegistryAuth sin la contraseña, y StackDeploy sin el compose (trae los
// valores de las variables) y con sus registries ocultos. Así un slog, un %v
// o un json.Marshal nunca los filtran. Los campos no cambian: dockerConfig,
// encodeRegistryAuth y el fake los leen directamente.

// hiddenPassword reemplaza una contraseña que no es vacía.
const hiddenPassword = "[oculta]"

func hide(password string) string {
	if password == "" {
		return ""
	}
	return hiddenPassword
}

// composeSize describe el compose sin mostrarlo.
func composeSize(b []byte) string { return fmt.Sprintf("(%d bytes)", len(b)) }

// String es {Host:ghcr.io Username:bot Password:[oculta]}.
func (a RegistryAuth) String() string {
	return fmt.Sprintf("{Host:%s Username:%s Password:%s}", a.Host, a.Username, hide(a.Password))
}

// GoString es String para %#v, con la forma de Go.
func (a RegistryAuth) GoString() string {
	return fmt.Sprintf("docker.RegistryAuth{Host:%q, Username:%q, Password:%q}", a.Host, a.Username, hide(a.Password))
}

// LogValue es un grupo con host, username y password oculta.
func (a RegistryAuth) LogValue() slog.Value {
	return slog.GroupValue(slog.String("host", a.Host), slog.String("username", a.Username),
		slog.String("password", hide(a.Password)))
}

// MarshalJSON serializa un struct anónimo con los mismos campos y la
// contraseña oculta ("" si está vacía). slog solo llama a LogValue en el
// valor de primer nivel: un struct que lleva un RegistryAuth adentro lo
// serializa el JSONHandler con encoding/json.
func (a RegistryAuth) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct{ Host, Username, Password string }{a.Host, a.Username, hide(a.Password)})
}

// String es {Name:p_s_a Compose:(1234 bytes) Registries:[…]}, con los
// registries ocultos.
func (d StackDeploy) String() string {
	regs := make([]string, len(d.Registries))
	for i, a := range d.Registries {
		regs[i] = a.String()
	}
	return fmt.Sprintf("{Name:%s Compose:%s Registries:[%s]}", d.Name, composeSize(d.Compose), strings.Join(regs, " "))
}

// GoString es String para %#v, con la forma de Go.
func (d StackDeploy) GoString() string {
	regs := make([]string, len(d.Registries))
	for i, a := range d.Registries {
		regs[i] = a.GoString()
	}
	return fmt.Sprintf("docker.StackDeploy{Name:%q, Compose:%s, Registries:[]docker.RegistryAuth{%s}}",
		d.Name, composeSize(d.Compose), strings.Join(regs, ", "))
}

// LogValue es un grupo con name, compose_bytes y los registries ocultos.
func (d StackDeploy) LogValue() slog.Value {
	return slog.GroupValue(slog.String("name", d.Name), slog.Int("compose_bytes", len(d.Compose)),
		slog.Any("registries", d.Registries))
}

// MarshalJSON serializa un struct anónimo: el compose como su tamaño y los
// registries con su MarshalJSON.
func (d StackDeploy) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Name       string
		Compose    string
		Registries []RegistryAuth
	}{d.Name, composeSize(d.Compose), d.Registries})
}
