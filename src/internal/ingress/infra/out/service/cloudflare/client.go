// Package cloudflare es el cliente mínimo de la API v4 de Cloudflare que usa
// ingress (implementa service.DNS): verificar el token, listar las zonas y
// manejar los registros DNS. Sin SDK: son seis rutas.
//
// El token va solo en el header Authorization de cada petición: nunca en la
// URL, en un error ni en un log. El cliente no sigue redirects, así el token
// nunca sale hacia otro host.
package cloudflare

import (
	"bytes"
	"context"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/CPU-commits/muelle/src/internal/ingress/domain/service"
	"github.com/CPU-commits/muelle/src/shared/domain/entity"
)

const (
	// DefaultBaseURL es la API real. Solo producción la usa: los tests pasan
	// la URL de cftest.
	DefaultBaseURL = "https://api.cloudflare.com/client/v4"
	// RecordComment va en el comentario de cada registro que crea muelle,
	// para reconocerlos en el panel de Cloudflare (contrato §3.1).
	RecordComment = "muelle"

	timeout = 15 * time.Second
	// maxBody es el máximo que se lee de una respuesta.
	maxBody = 1 << 20
	// maxPages corta una paginación que no termina.
	maxPages = 1000
	// Por página: /zones acepta hasta 50; los registros de un nombre son pocos.
	zonesPerPage   = 50
	recordsPerPage = 100
)

// Client es el cliente de Cloudflare.
type Client struct {
	base string
	http *http.Client
}

var _ service.DNS = (*Client)(nil)

// New arma el cliente con la base de la API ("" → DefaultBaseURL).
func New(baseURL string) *Client {
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	return &Client{
		base: strings.TrimSuffix(baseURL, "/"),
		http: &http.Client{
			Timeout: timeout,
			// Un redirect no se sigue: el 3xx se trata como una respuesta que
			// no se entiende.
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
	}
}

type apiError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type envelope struct {
	Success    bool           `json:"success"`
	Errors     []apiError     `json:"errors"`
	Result     jsontext.Value `json:"result"`
	ResultInfo *struct {
		TotalPages int `json:"total_pages"`
	} `json:"result_info"`
}

type zoneJSON struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type recordJSON struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Name    string `json:"name"`
	Content string `json:"content"`
	Proxied bool   `json:"proxied"`
}

func (r recordJSON) record() service.Record {
	return service.Record{ID: r.ID, Type: r.Type, Name: r.Name, Content: r.Content, Proxied: r.Proxied}
}

// unavailable es un error que envuelve entity.ErrCloudflareUnavailable.
func unavailable(format string, args ...any) error {
	return fmt.Errorf("%w: %s", entity.ErrCloudflareUnavailable, fmt.Sprintf(format, args...))
}

// validToken: un token de Cloudflare es ASCII visible. Uno con espacios o
// saltos de línea no se manda (no cabe en un header).
func validToken(token string) bool {
	if token == "" {
		return false
	}
	for i := 0; i < len(token); i++ {
		if token[i] < 0x21 || token[i] > 0x7e {
			return false
		}
	}
	return true
}

// do arma la petición con el token, limita la respuesta a 1 MiB, lee el sobre
// {success, errors, result, result_info} y clasifica: sin respuesta, 5xx, 429
// o JSON ilegible → ErrCloudflareUnavailable; 404 → ErrDNSNotFound (envolviendo
// el *DNSError); otro 4xx o success:false → *DNSError. out recibe result
// (nil: no se lee); pages es result_info.total_pages (0 si no viene).
func (c *Client) do(ctx context.Context, token, method, path string, q url.Values, body, out any) (pages int, err error) {
	if !validToken(token) {
		return 0, &service.DNSError{Status: http.StatusBadRequest, Message: "el token no es válido: tiene que ser texto ASCII sin espacios"}
	}
	u := c.base + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return 0, fmt.Errorf("Cloudflare: armando el pedido: %w", err)
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, rd)
	if err != nil {
		return 0, fmt.Errorf("Cloudflare: armando el pedido: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "muelle")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		// El texto de err trae la URL (sin el token: va en el header).
		return 0, fmt.Errorf("%w: sin respuesta: %w", entity.ErrCloudflareUnavailable, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxBody+1))
	if err != nil {
		return 0, fmt.Errorf("%w: leyendo la respuesta: %w", entity.ErrCloudflareUnavailable, err)
	}
	if len(raw) > maxBody {
		return 0, unavailable("la respuesta supera 1 MiB")
	}
	switch {
	case resp.StatusCode >= 500:
		return 0, unavailable("respondió %d", resp.StatusCode)
	case resp.StatusCode == http.StatusTooManyRequests:
		return 0, unavailable("respondió 429: demasiadas peticiones, reintenta en unos minutos")
	case resp.StatusCode < 200 || (resp.StatusCode >= 300 && resp.StatusCode < 400):
		return 0, unavailable("respondió %d", resp.StatusCode)
	}

	var env envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return 0, unavailable("la respuesta (%d) no se entiende", resp.StatusCode)
	}
	if resp.StatusCode >= 400 || !env.Success {
		de := rejected(resp.StatusCode, env.Errors, token)
		if resp.StatusCode == http.StatusNotFound {
			return 0, fmt.Errorf("%w: %w", service.ErrDNSNotFound, de)
		}
		return 0, de
	}
	if out != nil {
		if len(env.Result) == 0 || env.Result.Kind() == 'n' {
			return 0, unavailable("la respuesta no trae el resultado")
		}
		if err := json.Unmarshal(env.Result, out); err != nil {
			return 0, unavailable("el resultado no se entiende")
		}
	}
	if env.ResultInfo != nil {
		pages = env.ResultInfo.TotalPages
	}
	return pages, nil
}

// rejected arma el *DNSError con los errores de Cloudflare. Si algún mensaje
// trajera el token, va como ***.
func rejected(status int, errs []apiError, token string) *service.DNSError {
	de := &service.DNSError{Status: status}
	msgs := make([]string, 0, len(errs))
	for _, e := range errs {
		if de.Code == 0 {
			de.Code = e.Code
		}
		if e.Message != "" {
			msgs = append(msgs, e.Message)
		}
	}
	de.Message = strings.Join(msgs, "; ")
	if de.Message == "" {
		de.Message = http.StatusText(status)
	}
	de.Message = strings.ReplaceAll(de.Message, token, "***")
	return de
}

// op le pone a err la operación que falló.
func op(name string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("Cloudflare, %s: %w", name, err)
}

func zonePath(zoneID string) string { return "/zones/" + url.PathEscape(zoneID) + "/dns_records" }

func recordPath(zoneID, id string) string { return zonePath(zoneID) + "/" + url.PathEscape(id) }

// VerifyToken verifica el token. GET /user/tokens/verify responde 200 también
// con un token desactivado o vencido: lo que cuenta es result.status (D31).
func (c *Client) VerifyToken(ctx context.Context, token string) error {
	var res struct {
		Status string `json:"status"`
	}
	if _, err := c.do(ctx, token, http.MethodGet, "/user/tokens/verify", nil, nil, &res); err != nil {
		return op("verificando el token", err)
	}
	switch res.Status {
	case "active":
		return nil
	case "disabled":
		return op("verificando el token", &service.DNSError{Status: http.StatusOK, Message: "el token está desactivado"})
	case "expired":
		return op("verificando el token", &service.DNSError{Status: http.StatusOK, Message: "el token está vencido"})
	}
	return op("verificando el token", &service.DNSError{Status: http.StatusOK, Message: fmt.Sprintf("el token está en estado %q", res.Status)})
}

// Zones lista todas las zonas que ve el token, página por página.
func (c *Client) Zones(ctx context.Context, token string) ([]service.Zone, error) {
	out := []service.Zone{}
	for page := 1; ; page++ {
		var res []zoneJSON
		pages, err := c.do(ctx, token, http.MethodGet, "/zones",
			url.Values{"page": {strconv.Itoa(page)}, "per_page": {strconv.Itoa(zonesPerPage)}}, nil, &res)
		if err != nil {
			return nil, op("listando las zonas", err)
		}
		for _, z := range res {
			out = append(out, service.Zone{ID: z.ID, Name: z.Name})
		}
		if page >= pages || len(res) == 0 {
			return out, nil
		}
		if page >= maxPages {
			return nil, op("listando las zonas", unavailable("más de %d páginas", maxPages))
		}
	}
}

// Records lista los registros de la zona con ese nombre exacto, de todos los
// tipos. Cloudflare compara el nombre sin importar mayúsculas; el cliente
// vuelve a filtrar por si acaso.
func (c *Client) Records(ctx context.Context, token, zoneID, name string) ([]service.Record, error) {
	out := []service.Record{}
	for page := 1; ; page++ {
		var res []recordJSON
		pages, err := c.do(ctx, token, http.MethodGet, zonePath(zoneID), url.Values{
			"name.exact": {name}, "page": {strconv.Itoa(page)}, "per_page": {strconv.Itoa(recordsPerPage)},
		}, nil, &res)
		if err != nil {
			return nil, op("buscando los registros de "+name, err)
		}
		for _, r := range res {
			if strings.EqualFold(r.Name, name) {
				out = append(out, r.record())
			}
		}
		if page >= pages || len(res) == 0 {
			return out, nil
		}
		if page >= maxPages {
			return nil, op("buscando los registros de "+name, unavailable("más de %d páginas", maxPages))
		}
	}
}

// Record lee un registro por id.
func (c *Client) Record(ctx context.Context, token, zoneID, id string) (service.Record, error) {
	var res recordJSON
	if _, err := c.do(ctx, token, http.MethodGet, recordPath(zoneID, id), nil, nil, &res); err != nil {
		return service.Record{}, op("leyendo un registro", err)
	}
	return res.record(), nil
}

// CreateRecord crea r con ttl automático (1) y el comentario "muelle".
func (c *Client) CreateRecord(ctx context.Context, token, zoneID string, r service.Record) (service.Record, error) {
	body := struct {
		Type    string `json:"type"`
		Name    string `json:"name"`
		Content string `json:"content"`
		TTL     int    `json:"ttl"`
		Proxied bool   `json:"proxied"`
		Comment string `json:"comment"`
	}{r.Type, r.Name, r.Content, 1, r.Proxied, RecordComment}
	var res recordJSON
	if _, err := c.do(ctx, token, http.MethodPost, zonePath(zoneID), nil, body, &res); err != nil {
		return service.Record{}, op("creando el registro "+r.Type+" de "+r.Name, err)
	}
	if res.ID == "" {
		return service.Record{}, op("creando el registro "+r.Type+" de "+r.Name, unavailable("la respuesta no trae el id"))
	}
	return res.record(), nil
}

// UpdateRecord manda solo los campos de c que no son nil (PATCH).
func (c *Client) UpdateRecord(ctx context.Context, token, zoneID, id string, ch service.RecordChange) (service.Record, error) {
	body := struct {
		Type    *string `json:"type,omitzero"`
		Content *string `json:"content,omitzero"`
		Proxied *bool   `json:"proxied,omitzero"`
	}{ch.Type, ch.Content, ch.Proxied}
	var res recordJSON
	if _, err := c.do(ctx, token, http.MethodPatch, recordPath(zoneID, id), nil, body, &res); err != nil {
		return service.Record{}, op("actualizando un registro", err)
	}
	return res.record(), nil
}

// DeleteRecord borra un registro.
func (c *Client) DeleteRecord(ctx context.Context, token, zoneID, id string) error {
	_, err := c.do(ctx, token, http.MethodDelete, recordPath(zoneID, id), nil, nil, nil)
	return op("borrando un registro", err)
}
