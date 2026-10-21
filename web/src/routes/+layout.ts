// SPA estricta: nada se renderiza en el servidor ni se prerenderiza. adapter-static
// emite un unico `200.html` y el router resuelve todas las rutas en el navegador.
export const ssr = false
export const prerender = false
