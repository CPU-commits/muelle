package install

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"strings"
	"time"
)

const (
	// traceURL responde, entre otras líneas, ip=<la IP con que llegó la petición>
	// (contrato §5.4 paso 4). Por IP: no depende del DNS del servidor.
	traceURL = "https://1.1.1.1/cdn-cgi/trace"
	// traceTimeout y maxTrace acotan la detección: es un aviso si falla.
	traceTimeout = 5 * time.Second
	maxTrace     = 4 << 10
)

// detectPublicIP lee la IPv4 pública de la línea ip= de url, con a lo más
// 5 s (o el plazo de ctx, si es menor) y 4 KiB de respuesta (contrato §5.4
// paso 4). Solo una IPv4 canónica: otra cosa (una IPv6, si la máquina sale
// por IPv6) cuenta como no detectada.
func detectPublicIP(ctx context.Context, client *http.Client, url string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, traceTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	res, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("%s respondió %s", url, res.Status)
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, maxTrace+1))
	if err != nil {
		return "", err
	}
	if len(body) > maxTrace {
		// Solo las líneas enteras: una ip= cortada podría parecer otra IP válida.
		body = body[:maxTrace]
		body = body[:bytes.LastIndexByte(body, '\n')+1]
	}
	for line := range strings.Lines(string(body)) {
		v, ok := strings.CutPrefix(strings.TrimRight(line, "\r\n"), "ip=")
		if !ok {
			continue
		}
		a, err := netip.ParseAddr(v)
		if err != nil || !a.Is4() || a.String() != v {
			return "", errors.New("la respuesta no trae una IPv4 (la IPv6 va con --public-ipv6)")
		}
		return v, nil
	}
	return "", errors.New("la respuesta no trae la línea ip=")
}
