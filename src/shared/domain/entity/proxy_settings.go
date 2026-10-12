package entity

import (
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
)

// Claves de la tabla settings de las fases 2, 3 y 5 (§5.1 del spec). Las
// escribe admin (PATCH /settings y la configuración inicial de install) salvo
// donde se indica, y las leen ingress y deploy desde sus adaptadores SQL. El
// valor de cada una se guarda en texto, con el formato de ProxySettings.Encode.
const (
	// SettingCloudflareToken va cifrado (encrypted=1) con la AAD
	// SettingCloudflareTokenAAD. Es de solo escritura: GET /settings solo dice si está.
	SettingCloudflareToken = "cloudflare_token"
	// SettingCloudflareZones es cuántas zonas ve el token, en decimal. La
	// escribe ingress cada vez que verifica el token; si no está, no se sabe.
	SettingCloudflareZones = "cloudflare_zones"
	// SettingPublicIP es la IPv4 pública (forma canónica) de los registros A.
	SettingPublicIP = "public_ip"
	// SettingPublicIPv6 es la IPv6 pública (RFC 5952) de los registros AAAA; vacía = sin AAAA.
	SettingPublicIPv6 = "public_ipv6"
	// SettingACMEEmail es el email de la cuenta ACME de Caddy.
	SettingACMEEmail = "acme_email"
	// SettingPanelDomain es el host del panel (sin esquema ni puerto). Solo lo
	// escribe la configuración inicial (MUELLE_INIT_PANEL_DOMAIN): tiene que
	// coincidir con el host de MUELLE_PANEL_ORIGIN.
	SettingPanelDomain = "panel_domain"
	// SettingPanelAllowedCIDRs es un arreglo JSON de prefijos normalizados
	// (validate.CIDRs); [] = sin restricción.
	SettingPanelAllowedCIDRs = "panel_allowed_cidrs"
	// SettingManagerNodeID es el id del nodo manager de Swarm. Lo escribe
	// deploy al arrancar (Docker Info) y es el storage_node_id por defecto (§6.2).
	SettingManagerNodeID = "manager_node_id"
	// SettingProxyHTTPPort y SettingProxyHTTPSPort son los puertos del host que
	// publica muelle-caddy, en decimal (§3.1).
	SettingProxyHTTPPort  = "proxy_http_port"
	SettingProxyHTTPSPort = "proxy_https_port"
	// SettingCaddyImage es la imagen de muelle-caddy (ref completa con tag).
	SettingCaddyImage = "caddy_image"
)

// SettingCloudflareTokenAAD liga el token cifrado a su fila (§12: settings:<clave>).
const SettingCloudflareTokenAAD = "settings:" + SettingCloudflareToken

// Los puertos por defecto del proxy.
const (
	DefaultProxyHTTPPort  = 80
	DefaultProxyHTTPSPort = 443
)

// ProxySettingKeys son las claves que guarda ProxySettings.Encode (todas
// menos el token, que va cifrado aparte).
var ProxySettingKeys = []string{
	SettingACMEEmail, SettingCaddyImage, SettingPanelAllowedCIDRs, SettingPanelDomain,
	SettingProxyHTTPPort, SettingProxyHTTPSPort, SettingPublicIP, SettingPublicIPv6,
}

// ProxySettings son los ajustes de los que dependen el proxy y el DNS, ya
// decodificados: el contrato entre admin, que los guarda y valida, e ingress,
// que los aplica (admin/domain/service.SettingsHook). Un string vacío es "sin
// valor". El token va en claro: nunca se imprime (String y GoString lo ocultan).
type ProxySettings struct {
	CloudflareToken   string
	PublicIP          string
	PublicIPv6        string
	ACMEEmail         string
	PanelDomain       string
	PanelAllowedCIDRs []string // nunca nil
	HTTPPort          int
	HTTPSPort         int
	CaddyImage        string
}

// DefaultProxySettings son los ajustes de una base sin nada guardado.
func DefaultProxySettings() ProxySettings {
	return ProxySettings{
		PanelAllowedCIDRs: []string{},
		HTTPPort:          DefaultProxyHTTPPort,
		HTTPSPort:         DefaultProxyHTTPSPort,
	}
}

// Enabled dice si ingress maneja muelle-caddy: solo con el dominio del panel
// y la imagen de Caddy, que configura install. En desarrollo, sin ellos,
// muelle no crea el contenedor ni toca los puertos del host.
func (p ProxySettings) Enabled() bool { return p.PanelDomain != "" && p.CaddyImage != "" }

// DecodeProxySettings arma los ajustes con los valores guardados (clave →
// valor en texto, como los tiene la tabla settings) y el token ya descifrado.
// Una clave que falta, o vacía, toma su valor por defecto. Un valor que no se
// entiende es un error que nombra la clave: no se adivina.
func DecodeProxySettings(raw map[string]string, token string) (ProxySettings, error) {
	p := DefaultProxySettings()
	p.CloudflareToken = token
	p.PublicIP = raw[SettingPublicIP]
	p.PublicIPv6 = raw[SettingPublicIPv6]
	p.ACMEEmail = raw[SettingACMEEmail]
	p.PanelDomain = raw[SettingPanelDomain]
	p.CaddyImage = raw[SettingCaddyImage]
	for key, dst := range map[string]*int{SettingProxyHTTPPort: &p.HTTPPort, SettingProxyHTTPSPort: &p.HTTPSPort} {
		v := raw[key]
		if v == "" {
			continue
		}
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 65535 {
			return ProxySettings{}, fmt.Errorf("settings: %s = %q no es un puerto", key, v)
		}
		*dst = n
	}
	if v := raw[SettingPanelAllowedCIDRs]; v != "" {
		var list []string
		if err := json.Unmarshal([]byte(v), &list); err != nil {
			return ProxySettings{}, fmt.Errorf("settings: %s = %q no es un arreglo JSON de rangos", SettingPanelAllowedCIDRs, v)
		}
		if list != nil {
			p.PanelAllowedCIDRs = list
		}
	}
	return p, nil
}

// Encode devuelve los ajustes como se guardan (una entrada por cada clave de
// ProxySettingKeys), sin el token.
func (p ProxySettings) Encode() map[string]string {
	cidrs := p.PanelAllowedCIDRs
	if cidrs == nil {
		cidrs = []string{}
	}
	list, _ := json.Marshal(cidrs) // un []string siempre se puede serializar
	return map[string]string{
		SettingPublicIP:          p.PublicIP,
		SettingPublicIPv6:        p.PublicIPv6,
		SettingACMEEmail:         p.ACMEEmail,
		SettingPanelDomain:       p.PanelDomain,
		SettingPanelAllowedCIDRs: string(list),
		SettingProxyHTTPPort:     strconv.Itoa(p.HTTPPort),
		SettingProxyHTTPSPort:    strconv.Itoa(p.HTTPSPort),
		SettingCaddyImage:        p.CaddyImage,
	}
}

// ChangedKeys devuelve, ordenadas, las claves cuyo valor es distinto en p y
// en old, incluido SettingCloudflareToken si cambió el token.
func (p ProxySettings) ChangedKeys(old ProxySettings) []string {
	var out []string
	a, b := p.Encode(), old.Encode()
	for _, k := range ProxySettingKeys {
		if a[k] != b[k] {
			out = append(out, k)
		}
	}
	if p.CloudflareToken != old.CloudflareToken {
		out = append(out, SettingCloudflareToken)
	}
	slices.Sort(out)
	return out
}

// String oculta el token: un ProxySettings en un log nunca lo muestra.
func (p ProxySettings) String() string {
	q := p
	if q.CloudflareToken != "" {
		q.CloudflareToken = "***"
	}
	type plain ProxySettings // sin métodos: no vuelve a llamar a String
	return fmt.Sprintf("%+v", plain(q))
}

// GoString es String para %#v.
func (p ProxySettings) GoString() string { return p.String() }
