package service

import "net/netip"

// maxIPKey es el largo máximo de la parte de IP de una clave de los límites
// de login. La IP sale de X-Real-IP, así que puede ser cualquier texto.
const maxIPKey = 64

// ipKey agrupa una IP como la cuentan los límites de login: una IPv4 sola
// (/32) y una IPv6 por su /64, porque un cliente IPv6 suele tener un /64
// entero y podría rotar direcciones para saltarse los límites. Una IPv4
// mapeada en IPv6 cuenta como la IPv4. Lo que no es una IP se usa tal cual,
// recortado a maxIPKey bytes.
func ipKey(ip string) string {
	addr, err := netip.ParseAddr(ip)
	if err != nil {
		if len(ip) > maxIPKey {
			ip = ip[:maxIPKey]
		}
		return ip
	}
	addr = addr.Unmap().WithZone("")
	if addr.Is4() {
		return addr.String()
	}
	return netip.PrefixFrom(addr, 64).Masked().String()
}
