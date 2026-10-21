package entity

// DNSRecord es un registro que necesita un dominio con los ajustes actuales
// (el campo records del contrato §3.1): los que maneja muelle o, sin zona,
// los que hay que crear a mano.
type DNSRecord struct{ Type, Name, Content string }

// Records son los registros de hostname: un A con ipv4 y un AAAA con ipv6,
// los que no estén vacíos. Sin IPs es [] (nunca nil: el JSON es una lista).
func Records(hostname, ipv4, ipv6 string) []DNSRecord {
	out := []DNSRecord{}
	if ipv4 != "" {
		out = append(out, DNSRecord{Type: "A", Name: hostname, Content: ipv4})
	}
	if ipv6 != "" {
		out = append(out, DNSRecord{Type: "AAAA", Name: hostname, Content: ipv6})
	}
	return out
}
