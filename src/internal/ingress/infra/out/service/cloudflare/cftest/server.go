// Package cftest es un Cloudflare falso para los tests de ingress: un
// servidor httptest con estado en memoria (tokens, zonas y registros DNS)
// que responde como la API v4 lo que usa muelle. Ningún test de muelle llega
// a la API real: todos usan este servidor (o un httptest propio).
//
// Lo que imita de Cloudflare:
//   - el sobre {success, errors, messages, result, result_info};
//   - un token desconocido recibe 401 con el código 1000;
//   - una zona o un registro que no existen reciben 404;
//   - la paginación de /zones (per_page hasta 50) y de los registros;
//   - un A o AAAA no conviven con un CNAME del mismo nombre, ni dos CNAME
//     (81053), y dos registros idénticos tampoco (81058). PutRecord no revisa
//     nada: siembra lo que el test quiera;
//   - Origin CA (origin_ca.go): POST /certificates firma el CSR con una CA de
//     prueba en memoria (OriginRoot) si el token tiene Zone → SSL and
//     Certificates → Edit en la zona (GrantSSL), y DELETE /certificates/{id}
//     lo revoca; GET /zones trae status y permissions de cada zona.
package cftest

import (
	"cmp"
	"encoding/json/v2"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/CPU-commits/muelle/src/internal/ingress/domain/service"
)

// BasePath es el prefijo de la API, como en https://api.cloudflare.com/client/v4.
const BasePath = "/client/v4"

// Estados de un token en /user/tokens/verify.
const (
	statusActive  = "active"
	statusAccount = "account" // de una cuenta: verify lo rechaza, pero lista zonas
)

type record struct {
	service.Record
	comment string
}

type failure struct {
	method, pathContains string
	status, code         int
	msg                  string
}

// Server es el Cloudflare falso. Todos sus métodos se pueden llamar mientras
// atiende peticiones.
type Server struct {
	srv *httptest.Server

	mu      sync.Mutex
	tokens  map[string]string // token → estado
	zones   map[string]string // id → nombre
	records map[string]map[string]*record
	fails   []failure
	down    bool
	hang    time.Duration
	calls   []string
	nextID  int
	origin  originCA
}

// New arranca el servidor; se cierra con t.Cleanup.
func New(t testing.TB) *Server {
	t.Helper()
	s := &Server{tokens: map[string]string{}, zones: map[string]string{}, records: map[string]map[string]*record{}}
	s.origin = newOriginCA(t)
	s.srv = httptest.NewServer(http.HandlerFunc(s.serve))
	t.Cleanup(s.srv.Close)
	return s
}

// URL es la base de la API (con BasePath), para Options.CloudflareURL y cloudflare.New.
func (s *Server) URL() string { return s.srv.URL + BasePath }

// AddToken agrega un token activo: /user/tokens/verify lo da "active".
func (s *Server) AddToken(token string) { s.setToken(token, statusActive) }

// AddAccountToken agrega un token de una cuenta: /user/tokens/verify responde
// 401 (se verifica en otra ruta), pero lista zonas y maneja registros.
func (s *Server) AddAccountToken(token string) { s.setToken(token, statusAccount) }

// SetTokenStatus deja el token en "disabled" o "expired": verify responde 200
// con ese status, y todo lo demás 401.
func (s *Server) SetTokenStatus(token, status string) { s.setToken(token, status) }

func (s *Server) setToken(token, status string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tokens[token] = status
}

// AddZone agrega una zona, sin registros.
func (s *Server) AddZone(id, name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.zones[id] = name
	if s.records[id] == nil {
		s.records[id] = map[string]*record{}
	}
}

// PutRecord guarda r en la zona (que tiene que existir), sin comentario y
// sin revisar conflictos, y devuelve su id: el de r, o uno nuevo si viene vacío.
func (s *Server) PutRecord(zoneID string, r service.Record) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	recs, ok := s.records[zoneID]
	if !ok {
		panic("cftest: PutRecord en una zona que no existe: " + zoneID)
	}
	if r.ID == "" {
		r.ID = s.newID()
	}
	recs[r.ID] = &record{Record: r}
	return r.ID
}

// Records es una copia de los registros de la zona, ordenada por nombre y tipo.
func (s *Server) Records(zoneID string) []service.Record {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []service.Record{}
	for _, r := range s.records[zoneID] {
		out = append(out, r.Record)
	}
	slices.SortFunc(out, func(a, b service.Record) int {
		return cmp.Or(cmp.Compare(a.Name, b.Name), cmp.Compare(a.Type, b.Type), cmp.Compare(a.ID, b.ID))
	})
	return out
}

// Comment es el comentario guardado del registro ("" si no tiene o no existe).
func (s *Server) Comment(zoneID, id string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if r, ok := s.records[zoneID][id]; ok {
		return r.comment
	}
	return ""
}

// SetDown hace que todo responda 503, con un cuerpo HTML como el de un corte real.
func (s *Server) SetDown(down bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.down = down
}

// FailNext hace que la próxima petición con ese método y cuyo path (con la
// query, sin BasePath) contiene pathContains responda status con un error
// {code, msg}, una sola vez. Con un status 2xx responde success: false.
func (s *Server) FailNext(method, pathContains string, status, code int, msg string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.fails = append(s.fails, failure{method, pathContains, status, code, msg})
}

// Hang demora cada respuesta d (0: sin demora). Una petición cancelada deja
// de esperar.
func (s *Server) Hang(d time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.hang = d
}

// Calls son las peticiones recibidas, en orden: el método y el path sin
// BasePath, con la query como llegó ("GET /zones?page=1&per_page=50",
// "POST /zones/z1/dns_records"…).
func (s *Server) Calls() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.calls)
}

func (s *Server) newID() string {
	s.nextID++
	return fmt.Sprintf("%032x", s.nextID)
}

// envelope es el sobre de todas las respuestas de la API v4.
type envelope struct {
	Success    bool        `json:"success"`
	Errors     []apiError  `json:"errors"`
	Messages   []apiError  `json:"messages"`
	Result     any         `json:"result"`
	ResultInfo *resultInfo `json:"result_info,omitzero"`
}

type apiError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type resultInfo struct {
	Page       int `json:"page"`
	PerPage    int `json:"per_page"`
	Count      int `json:"count"`
	TotalCount int `json:"total_count"`
	TotalPages int `json:"total_pages"`
}

type recordJSON struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Name     string `json:"name"`
	Content  string `json:"content"`
	Proxied  bool   `json:"proxied"`
	TTL      int    `json:"ttl"`
	Comment  any    `json:"comment"`
	ZoneID   string `json:"zone_id"`
	ZoneName string `json:"zone_name"`
}

func writeJSON(w http.ResponseWriter, status int, v envelope) {
	if v.Errors == nil {
		v.Errors = []apiError{}
	}
	v.Messages = []apiError{}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.MarshalWrite(w, v)
}

func fail(w http.ResponseWriter, status, code int, msg string) {
	writeJSON(w, status, envelope{Errors: []apiError{{Code: code, Message: msg}}})
}

func ok(w http.ResponseWriter, result any, info *resultInfo) {
	writeJSON(w, http.StatusOK, envelope{Success: true, Result: result, ResultInfo: info})
}

func (s *Server) serve(w http.ResponseWriter, r *http.Request) {
	// Los segmentos salen del path escapado: un id con "/" escapado sigue
	// siendo un solo segmento.
	escaped, found := strings.CutPrefix(r.URL.EscapedPath(), BasePath)
	parts := strings.Split(strings.Trim(escaped, "/"), "/")
	for i, p := range parts {
		if u, err := url.PathUnescape(p); err == nil {
			parts[i] = u
		}
	}
	call := strings.TrimPrefix(r.URL.RequestURI(), BasePath)

	s.mu.Lock()
	s.calls = append(s.calls, r.Method+" "+call)
	hang, down := s.hang, s.down
	var injected *failure
	for i, f := range s.fails {
		if f.method == r.Method && strings.Contains(call, f.pathContains) {
			injected = &f
			s.fails = slices.Delete(s.fails, i, i+1)
			break
		}
	}
	s.mu.Unlock()

	if hang > 0 {
		select {
		case <-time.After(hang):
		case <-r.Context().Done():
			return
		}
	}
	switch {
	case down:
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusServiceUnavailable)
		fmt.Fprint(w, "<html><body>503 Service Temporarily Unavailable</body></html>")
		return
	case injected != nil:
		fail(w, injected.status, injected.code, injected.msg)
		return
	case !found:
		fail(w, http.StatusNotFound, 7000, "No route for that URI")
		return
	}

	token, _ := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	s.mu.Lock()
	defer s.mu.Unlock()
	status, known := s.tokens[token]
	if slices.Equal(parts, []string{"user", "tokens", "verify"}) && r.Method == http.MethodGet {
		if !known || status == statusAccount {
			fail(w, http.StatusUnauthorized, 1000, "Invalid API Token")
			return
		}
		ok(w, map[string]string{"id": "tok-" + strconv.Itoa(len(token)), "status": status}, nil)
		return
	}
	if !known || (status != statusActive && status != statusAccount) {
		fail(w, http.StatusUnauthorized, 1000, "Invalid API Token")
		return
	}

	switch {
	case len(parts) == 1 && parts[0] == "zones" && r.Method == http.MethodGet:
		s.listZones(w, r, token)
	case len(parts) == 1 && parts[0] == "certificates" && r.Method == http.MethodPost:
		s.createOriginCert(w, r, token)
	case len(parts) == 2 && parts[0] == "certificates" && r.Method == http.MethodDelete:
		s.revokeOriginCert(w, token, parts[1])
	case len(parts) >= 3 && parts[0] == "zones" && parts[2] == "dns_records":
		zoneID := parts[1]
		if _, ok := s.zones[zoneID]; !ok {
			fail(w, http.StatusNotFound, 7003, "Could not route to /zones/"+zoneID+"/dns_records, perhaps your object identifier is invalid?")
			return
		}
		switch {
		case len(parts) == 3 && r.Method == http.MethodGet:
			s.listRecords(w, r, zoneID)
		case len(parts) == 3 && r.Method == http.MethodPost:
			s.createRecord(w, r, zoneID)
		case len(parts) == 4:
			s.recordByID(w, r, zoneID, parts[3])
		default:
			fail(w, http.StatusMethodNotAllowed, 10000, "Method not allowed")
		}
	default:
		fail(w, http.StatusNotFound, 7000, "No route for that URI")
	}
}

// paginate devuelve la página pedida de n elementos y su result_info.
func paginate(r *http.Request, n, defPerPage, maxPerPage int) (from, to int, info *resultInfo) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	page = max(page, 1)
	perPage, err := strconv.Atoi(r.URL.Query().Get("per_page"))
	if err != nil || perPage < 1 {
		perPage = defPerPage
	}
	perPage = min(perPage, maxPerPage)
	from = min((page-1)*perPage, n)
	to = min(from+perPage, n)
	pages := (n + perPage - 1) / perPage
	return from, to, &resultInfo{Page: page, PerPage: perPage, Count: to - from, TotalCount: n, TotalPages: pages}
}

func (s *Server) listZones(w http.ResponseWriter, r *http.Request, token string) {
	ids := make([]string, 0, len(s.zones))
	for id := range s.zones {
		ids = append(ids, id)
	}
	slices.SortFunc(ids, func(a, b string) int { return cmp.Or(cmp.Compare(s.zones[a], s.zones[b]), cmp.Compare(a, b)) })
	from, to, info := paginate(r, len(ids), 20, 50)
	out := []map[string]any{}
	for _, id := range ids[from:to] {
		z := map[string]any{"id": id, "name": s.zones[id], "status": s.zoneStatus(id)}
		if !s.origin.omitPerms {
			z["permissions"] = s.zonePermissions(token, id)
		}
		out = append(out, z)
	}
	ok(w, out, info)
}

func (s *Server) toJSON(zoneID string, rec *record) recordJSON {
	var comment any
	if rec.comment != "" {
		comment = rec.comment
	}
	return recordJSON{
		ID: rec.ID, Type: rec.Type, Name: rec.Name, Content: rec.Content, Proxied: rec.Proxied,
		TTL: 1, Comment: comment, ZoneID: zoneID, ZoneName: s.zones[zoneID],
	}
}

func (s *Server) sorted(zoneID string) []*record {
	out := []*record{}
	for _, rec := range s.records[zoneID] {
		out = append(out, rec)
	}
	slices.SortFunc(out, func(a, b *record) int {
		return cmp.Or(cmp.Compare(a.Name, b.Name), cmp.Compare(a.Type, b.Type), cmp.Compare(a.ID, b.ID))
	})
	return out
}

func (s *Server) listRecords(w http.ResponseWriter, r *http.Request, zoneID string) {
	q := r.URL.Query()
	name, typ := q.Get("name.exact"), q.Get("type")
	var match []*record
	for _, rec := range s.sorted(zoneID) {
		if (name == "" || strings.EqualFold(rec.Name, name)) && (typ == "" || rec.Type == typ) {
			match = append(match, rec)
		}
	}
	from, to, info := paginate(r, len(match), 100, 5000)
	out := []recordJSON{}
	for _, rec := range match[from:to] {
		out = append(out, s.toJSON(zoneID, rec))
	}
	ok(w, out, info)
}

type recordBody struct {
	Type    *string `json:"type"`
	Name    *string `json:"name"`
	Content *string `json:"content"`
	Proxied *bool   `json:"proxied"`
	TTL     *int    `json:"ttl"`
	Comment *string `json:"comment"`
}

// conflict es el error de Cloudflare si r no puede convivir con los demás
// registros de la zona (salvo el de id skip).
func (s *Server) conflict(zoneID string, r service.Record, skip string) (code int, msg string) {
	for _, o := range s.records[zoneID] {
		if o.ID == skip || !strings.EqualFold(o.Name, r.Name) {
			continue
		}
		if o.Type == r.Type && o.Content == r.Content {
			return 81058, "An identical record already exists."
		}
		addr := func(t string) bool { return t == "A" || t == "AAAA" || t == "CNAME" }
		if (r.Type == "CNAME" && addr(o.Type)) || (o.Type == "CNAME" && addr(r.Type)) {
			return 81053, "An A, AAAA, or CNAME record with that host already exists."
		}
	}
	return 0, ""
}

func (s *Server) createRecord(w http.ResponseWriter, r *http.Request, zoneID string) {
	var b recordBody
	if err := json.UnmarshalRead(r.Body, &b); err != nil {
		fail(w, http.StatusBadRequest, 9207, "Request body is invalid.")
		return
	}
	if b.Type == nil || b.Name == nil || b.Content == nil {
		fail(w, http.StatusBadRequest, 9000, "DNS record type, name and content are required.")
		return
	}
	rec := &record{Record: service.Record{Type: *b.Type, Name: strings.ToLower(*b.Name), Content: *b.Content}}
	if b.Proxied != nil {
		rec.Proxied = *b.Proxied
	}
	if b.Comment != nil {
		rec.comment = *b.Comment
	}
	if code, msg := s.conflict(zoneID, rec.Record, ""); code != 0 {
		fail(w, http.StatusBadRequest, code, msg)
		return
	}
	rec.ID = s.newID()
	s.records[zoneID][rec.ID] = rec
	ok(w, s.toJSON(zoneID, rec), nil)
}

func (s *Server) recordByID(w http.ResponseWriter, r *http.Request, zoneID, id string) {
	rec, found := s.records[zoneID][id]
	if !found {
		fail(w, http.StatusNotFound, 81044, "Record does not exist.")
		return
	}
	switch r.Method {
	case http.MethodGet:
		ok(w, s.toJSON(zoneID, rec), nil)
	case http.MethodDelete:
		delete(s.records[zoneID], id)
		ok(w, map[string]string{"id": id}, nil)
	case http.MethodPatch:
		var b recordBody
		if err := json.UnmarshalRead(r.Body, &b); err != nil {
			fail(w, http.StatusBadRequest, 9207, "Request body is invalid.")
			return
		}
		next := *rec
		if b.Type != nil {
			next.Type = *b.Type
		}
		if b.Name != nil {
			next.Name = strings.ToLower(*b.Name)
		}
		if b.Content != nil {
			next.Content = *b.Content
		}
		if b.Proxied != nil {
			next.Proxied = *b.Proxied
		}
		if b.Comment != nil {
			next.comment = *b.Comment
		}
		if code, msg := s.conflict(zoneID, next.Record, id); code != 0 {
			fail(w, http.StatusBadRequest, code, msg)
			return
		}
		*rec = next
		ok(w, s.toJSON(zoneID, rec), nil)
	default:
		fail(w, http.StatusMethodNotAllowed, 10000, "Method not allowed")
	}
}
