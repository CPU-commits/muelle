#!/usr/bin/env bash
# muelle/action (diseño §14): sincroniza las variables de una app de muelle, crea
# un deploy y sigue su log hasta el resultado.
#
# Las entradas llegan solo como variables de entorno (las INPUT_* de action.yml),
# nunca dentro de un run:. Ningún secreto va en la línea de comandos de un
# proceso, que cualquiera en el runner ve en /proc/<pid>/cmdline:
#   - el token va en un archivo de headers 0600 (curl -H @archivo);
#   - env-json lo lee jq del entorno (env.INPUT_ENV_JSON) y el cuerpo del PUT
#     va en un archivo (--data-binary @archivo).
# Lo que responde muelle y va a una URL o a $GITHUB_OUTPUT se valida antes.
set -euo pipefail
export LC_ALL=C # las expresiones regulares comparan bytes: [!-~] es solo ASCII visible

# Solo para los tests (action_test.go); no son entradas de la action.
poll="${MUELLE_ACTION_TEST_POLL:-3}"
read -r -a retry_delays <<<"${MUELLE_ACTION_TEST_RETRY_DELAYS:-2 4}"

tmp=""
log_token="" # el de ::stop-commands::, mientras el log está abierto
line_open=0  # 1 si lo último del log no terminó en \n
status=""    # el status HTTP de la última petición: 000 si no hubo respuesta

# escape: un mensaje de ::error:: o ::warning::, escapado como pide GitHub.
escape() {
	local s=$1
	s=${s//'%'/'%25'}
	s=${s//$'\r'/'%0D'}
	s=${s//$'\n'/'%0A'}
	printf '%s' "$s"
}

# end_log cierra el bloque del log, una sola vez: desde ahí el runner vuelve a
# leer los comandos (::error::, ::warning::).
end_log() {
	if [[ -n $log_token ]]; then
		if [[ $line_open == 1 ]]; then printf '\n'; fi
		printf '::%s::\n' "$log_token"
		log_token=""
	fi
}

cleanup() {
	end_log
	if [[ -n $tmp ]]; then rm -rf "$tmp"; fi
}

fail() {
	end_log
	printf '::error::%s\n' "$(escape "$1")"
	exit 1
}

warn() { printf '::warning::%s\n' "$(escape "$1")"; }

# output solo recibe valores ya validados (want_id, want_status): un valor con
# \n agregaría otras salidas.
output() { printf '%s=%s\n' "$1" "$2" >>"${GITHUB_OUTPUT:-/dev/null}"; }

# want_id CAMPO VALOR: un entero. El error nombra el campo y no repite el valor.
want_id() {
	if ! [[ $2 =~ ^[0-9]{1,15}$ ]]; then
		fail "muelle respondió un campo $1 inválido (tiene que ser un entero)"
	fi
}

# want_status VALOR: uno de los estados de un deploy (contrato §2.1).
want_status() {
	case $1 in
	queued | running | succeeded | failed | superseded) ;;
	*) fail "muelle respondió un campo status inválido (no es un estado de deploy)" ;;
	esac
}

# field CAMPO: el campo de primer nivel de la última respuesta, como texto.
field() { jq -r --arg f "$1" '.[$f] | tostring' "$tmp/body" 2>/dev/null; }

# request MÉTODO RUTA [ARCHIVO]: pide $url$RUTA con el token. Deja el cuerpo en
# $tmp/body, los headers en $tmp/headers (el Retry-After de un 429) y el status
# en $status (000 sin respuesta). --globoff: los [] y {} de una ruta no son
# patrones de curl. Sin -L: una redirección nunca lleva el token a otro lado.
request() {
	local body=()
	if [[ -n ${3:-} ]]; then body=(-H 'Content-Type: application/json' --data-binary "@$3"); fi
	: >"$tmp/body"
	: >"$tmp/headers"
	status=$(curl --globoff -sS --max-time 30 -o "$tmp/body" -D "$tmp/headers" -w '%{http_code}' \
		-X "$1" -H "@$tmp/auth" -H 'Accept: application/json' -H 'Expect:' \
		${body[@]+"${body[@]}"} "$url$2" 2>"$tmp/curl.err") || status=000
}

transient() { [[ $status == 000 || $status == 5?? || $status == 429 ]]; }

curl_error() {
	local line=""
	IFS= read -r line <"$tmp/curl.err" || true
	printf '%s' "${line:-sin respuesta}"
}

# api_error QUÉ: el error de la última respuesta, con el code y el message de
# muelle (contrato §0.2).
api_error() {
	local code="" message=""
	if [[ $status == 000 ]]; then
		printf '%s: muelle no respondió (%s)' "$1" "$(curl_error)"
		return 0
	fi
	code=$(jq -r '.error.code | strings' "$tmp/body" 2>/dev/null) || code=""
	message=$(jq -r '.error.message | strings' "$tmp/body" 2>/dev/null) || message=""
	if [[ $status == 404 && $code == no_route ]]; then
		printf '%s: este muelle todavía no tiene la API de deploys (404 no_route); actualízalo' "$1"
	elif [[ -n $code ]]; then
		printf '%s: %s %s: %s' "$1" "$status" "$code" "$message"
	else
		printf '%s: muelle respondió %s' "$1" "$status"
	fi
}

# retry_after: los segundos del Retry-After de la última respuesta, o nada si
# no viene o es una fecha HTTP.
retry_after() {
	local line
	line=$(grep -i -m1 '^retry-after:' "$tmp/headers" 2>/dev/null) || return 0
	line=${line#*:}
	line=${line//[[:space:]]/}
	if [[ $line =~ ^[0-9]{1,6}$ ]]; then printf '%s' "$((10#$line))"; fi
}

# put_env: PUT …/env, hasta 3 intentos si muelle no responde o responde 5xx o
# 429. Con un 429 espera lo que pide Retry-After si es más que el retraso que
# toca, hasta 60 s; si pide más, falla. Otro 4xx no se reintenta.
put_env() {
	local attempt delay ra
	for attempt in 1 2 3; do
		request PUT "$base/env" "$tmp/env.json"
		if [[ $status == 2?? ]]; then return 0; fi
		if [[ $attempt == 3 ]] || ! transient; then
			fail "$(api_error 'No se pudieron guardar las variables')"
		fi
		delay=${retry_delays[attempt - 1]:-0}
		if [[ $status == 429 ]]; then
			ra=$(retry_after)
			if [[ -n $ra ]] && ((ra > 60)); then
				fail "$(api_error 'No se pudieron guardar las variables') (pide esperar $ra s)"
			fi
			if [[ -n $ra ]] && ((ra > delay)); then delay=$ra; fi
		fi
		sleep "$delay"
	done
}

for tool in curl jq; do
	if ! command -v "$tool" >/dev/null; then fail "esta action necesita $tool en el runner"; fi
done

# 1. Las entradas, antes de tocar muelle.
url=${INPUT_URL:-}
url=${url%/}
re='^(https://([A-Za-z0-9.-]+|\[[0-9A-Fa-f:]+\])(:[0-9]{1,5})?|http://(localhost|127\.0\.0\.1)(:[0-9]{1,5})?)$'
if ! [[ $url =~ $re ]]; then
	fail "url tiene que ser el origen del panel de muelle, con https:// y sin ruta (https://panel.tudominio.cl); http:// solo con localhost o 127.0.0.1"
fi
re='^[!-~]+$'
if ! [[ ${INPUT_TOKEN:-} =~ $re ]]; then
	fail "token tiene que ser un token de API de muelle (mk_…), sin espacios, saltos de línea ni caracteres fuera de ASCII"
fi
app=${INPUT_APP:-}
re='^[a-z][a-z0-9-]*/[a-z][a-z0-9-]*/[a-z][a-z0-9-]*$'
if ! [[ $app =~ $re ]]; then
	fail "app tiene que ser proyecto/stage/app: tres slugs (minúsculas, números y -) separados por /"
fi
image=${INPUT_IMAGE:-}
images_json=${INPUT_IMAGES_JSON:-}
if [[ -n $image && -n $images_json ]]; then
	fail "image e images-json no van juntos: image es para una app de formulario, images-json para una compose"
fi
strings_object='fromjson | type == "object" and all(.[]; type == "string")'
if [[ -n $images_json ]] && ! jq -en "env.INPUT_IMAGES_JSON | $strings_object" >/dev/null 2>&1; then
	fail "images-json tiene que ser un objeto JSON de strings (servicio → imagen)"
fi
# El error de fromjson cita el texto entero, que serían los secretos: el stderr
# de jq se descarta.
has_env=$(jq -n '(env.INPUT_ENV_JSON // "") != ""')
if [[ $has_env == true ]] && ! jq -en "env.INPUT_ENV_JSON | $strings_object" >/dev/null 2>&1; then
	fail "env-json tiene que ser un objeto JSON de strings (por ejemplo toJson(secrets))"
fi
secret=${INPUT_ENV_SECRET:-true}
case $secret in
true | false) ;;
*) fail "env-secret tiene que ser true o false" ;;
esac
timeout=${INPUT_TIMEOUT:-600}
if ! [[ $timeout =~ ^[0-9]{1,9}$ ]] || ((10#$timeout == 0)); then
	fail "timeout tiene que ser un entero de segundos mayor que 0"
fi
timeout=$((10#$timeout))

umask 077
tmp=$(mktemp -d "${RUNNER_TEMP:-${TMPDIR:-/tmp}}/muelle-action.XXXXXXXX")
trap cleanup EXIT
trap 'exit 1' INT TERM
printf 'Authorization: Bearer %s\n' "$INPUT_TOKEN" >"$tmp/auth"
printf '::add-mask::%s\n' "$INPUT_TOKEN"

project=${app%%/*}
rest=${app#*/}
stage=${rest%%/*}
name=${rest#*/}
base="/api/v1/projects/$project/stages/$stage/apps/$name"

# 2. Las variables: PUT reemplaza todo el set de la app.
if [[ $has_env == true ]]; then
	if ! jq -n --arg secret "$secret" '
		([(env.INPUT_ENV_EXCLUDE // "") | split(",")[] | gsub("^\\s+|\\s+$"; "") | ascii_upcase | select(. != "")]
			+ ["GITHUB_TOKEN", "MUELLE_URL", "MUELLE_TOKEN"]) as $exclude
		| {vars: (env.INPUT_ENV_JSON | fromjson
			| with_entries(select((.key | ascii_upcase) as $k | any($exclude[]; . == $k) | not))),
		   secret: ($secret == "true")}' >"$tmp/env.json" 2>/dev/null; then
		fail "no se pudo filtrar env-json"
	fi
	put_env
	printf 'muelle: %s variables guardadas en %s\n' "$(jq '.vars | length' "$tmp/env.json")" "$app"
fi

# 3. El deploy, una sola vez: no es idempotente.
if [[ -n $image ]]; then
	jq -cn '{image: env.INPUT_IMAGE}' >"$tmp/deploy.json"
elif [[ -n $images_json ]]; then
	jq -cn '{images: (env.INPUT_IMAGES_JSON | fromjson)}' >"$tmp/deploy.json"
else
	printf '{}' >"$tmp/deploy.json"
fi
request POST "$base/deploys" "$tmp/deploy.json"
if [[ $status == 000 ]]; then
	fail "No se sabe si muelle creó el deploy: no respondió ($(curl_error)). No se reintenta, para no crear dos: revísalo en el panel"
fi
if [[ $status != 202 ]]; then fail "$(api_error 'No se pudo crear el deploy')"; fi
id=$(field id) || id=""
want_id id "$id"
last=$(field status) || last=""
want_status "$last"
status_url=$(field status_url) || status_url=""
re='^/api/v1/[!-~]+$'
if ! [[ $status_url =~ $re ]]; then
	fail "muelle respondió un campo status_url inválido (tiene que ser una ruta de /api/v1/: el token solo va a url)"
fi
output deploy-id "$id"
# SECONDS cuenta segundos enteros: con > (no >=) espera al menos timeout segundos.
deadline=$((SECONDS + timeout))
printf 'muelle: deploy #%s de %s creado (%s)\n' "$id" "$app" "$last"

# 4. El log, por offset, entre ::stop-commands:: y su cierre: lo que imprime la
# app nunca se lee como un comando del runner.
tok=$(head -c 16 /dev/urandom | od -An -tx1 | tr -d ' \n')
if ! [[ $tok =~ ^[0-9a-f]{32}$ ]]; then fail "no se pudo generar un token al azar"; fi
printf '::stop-commands::%s\n' "$tok"
log_token=$tok
offset=0
superseded_by=null
while :; do
	advanced=0
	request GET "$status_url/log?offset=$offset"
	if [[ $status == 200 ]]; then
		next=$(field next_offset) || next=""
		want_id next_offset "$next"
		st=$(field status) || st=""
		want_status "$st"
		superseded_by=$(field superseded_by) || superseded_by=""
		if [[ $superseded_by != null ]]; then want_id superseded_by "$superseded_by"; fi
		if ! jq -j '.chunk | if type == "string" then . else error("chunk") end' "$tmp/body" >"$tmp/chunk" 2>/dev/null; then
			fail "muelle respondió un campo chunk inválido (tiene que ser un string)"
		fi
		if [[ -s $tmp/chunk ]]; then
			cat "$tmp/chunk"
			if [[ $(tail -c 1 "$tmp/chunk" && printf x) == $'\nx' ]]; then line_open=0; else line_open=1; fi
		fi
		last=$st
		if ((10#$next > offset)); then
			offset=$((10#$next))
			advanced=1
		elif [[ $st == succeeded || $st == failed || $st == superseded ]]; then
			break
		fi
	elif ! transient; then
		fail "$(api_error 'No se pudo leer el log del deploy')"
	fi
	if ((SECONDS > deadline)); then
		output status "$last"
		fail "timeout: el deploy #$id seguía en $last después de $timeout s. El deploy sigue en muelle: revísalo en el panel"
	fi
	if ((advanced == 0)); then sleep "$poll"; fi
done

# 5. El resultado.
end_log
output status "$last"
case $last in
succeeded)
	printf 'muelle: el deploy #%s terminó bien\n' "$id"
	;;
superseded)
	if [[ $superseded_by != null ]]; then
		warn "El deploy #$id no corrió (superseded): lo reemplazó el #$superseded_by, más nuevo, de la misma app. El resultado es el de ese deploy"
	else
		warn "El deploy #$id no corrió (superseded): lo reemplazó uno más nuevo de la misma app"
	fi
	;;
failed)
	deploy_error=""
	for _ in 1 2 3; do
		request GET "$status_url"
		if [[ $status == 200 ]]; then
			deploy_error=$(jq -r '.error | strings' "$tmp/body" 2>/dev/null) || deploy_error=""
			break
		fi
		if ! transient; then break; fi
		sleep "$poll"
	done
	fail "El deploy #$id falló: ${deploy_error:-muelle no dijo por qué (revísalo en el panel)}"
	;;
esac
