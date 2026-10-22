// Cómo conectar Claude Code con el servidor MCP de muelle (contrato §8.7): la URL es el
// origen en que se abrió el panel más `/api/v1/mcp`, con su puerto si lo tiene.

const MCP_PATH = '/api/v1/mcp'

/** `https://<panel>[:puerto]/api/v1/mcp`. */
export function mcpServerUrl(origin: string): string {
	return new URL(MCP_PATH, origin).href
}

/**
 * `claude mcp add` con el token (lleva el secreto). El `--header` va después de la URL, como
 * en la documentación de Claude Code.
 */
export function claudeAddCommand(url: string, secret: string): string {
	return [
		'claude mcp add --transport http --scope user muelle',
		url,
		`--header "Authorization: Bearer ${secret}"`,
	].join(' ')
}

/**
 * Un `.mcp.json` para el repositorio: el token no va en el archivo, cada persona exporta
 * `MUELLE_TOKEN` en su shell (Claude Code expande `${…}` en los headers).
 */
export function mcpJsonSnippet(url: string): string {
	return [
		'{',
		'  "mcpServers": {',
		'    "muelle": {',
		'      "type": "http",',
		`      "url": ${JSON.stringify(url)},`,
		'      "headers": { "Authorization": "Bearer ${MUELLE_TOKEN}" }',
		'    }',
		'  }',
		'}',
	].join('\n')
}
