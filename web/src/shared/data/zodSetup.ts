import { z } from 'zod'

// Zod 4 compila cada `z.object` con `new Function` para parsear más rápido, y para saber
// si puede lo prueba una vez. La CSP del panel no tiene 'unsafe-eval': la prueba falla, y
// el navegador la reporta como violación aunque Zod se recupere solo. Con `jitless` Zod
// ni lo intenta.
//
// Zod decide al CREAR cada schema, no al parsear: este módulo tiene que evaluarse antes
// que cualquier `z.object`. Por eso es el primer import de hooks.client.ts y de config.ts.
z.config({ jitless: true })
