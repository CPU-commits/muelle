// Geist y Geist Mono, variables (un archivo por subconjunto cubre 400 a 600). Vite
// copia los woff2 al build y el panel los sirve desde el binario: ningún pedido a
// Google Fonts ni a un CDN. El navegador baja solo los subconjuntos que usa la página
// (`unicode-range`). Se importa una vez, en el layout raíz.
import '@fontsource-variable/geist'
import '@fontsource-variable/geist-mono'
