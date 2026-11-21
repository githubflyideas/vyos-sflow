---
title: ¿Quién se está comiendo mi tráfico de VyOS? Esta herramientita lo muestra de un vistazo 😭
date: 2026-11-24
summary: ¿La red de la oficina se arrastra en horas pico y nadie sabe quién se come el ancho de banda? Un solo binario lo arregla: ancho de banda, top talkers y registros de flujo, para todo lo que exporte sFlow, en el navegador o por SSH. Funcionando en 30 segundos.
---

Primero, el dolor 👇

El VyOS de la salida de nuestra oficina lleva mucho tiempo encendido.
Cada día laborable, en hora pico, la red se arrastra.
¿Quién se come el ancho de banda? Ni idea.
¿Qué máquina, hablando con qué servidor? Tampoco.
Puro adivinar. La respuesta siempre era «prueba reiniciando» 🙂

VyOS puede exportar muestras sFlow.
También la mayoría de los switches, routers de otras marcas y servidores.

¿Y después qué? No había nada con qué verlas ❌
Las herramientas de siempre piden otro servidor, o una base de datos con todo un sistema de paneles.
Demasiado, solo para mirar mi propio tráfico.

Así que escribí una herramientita yo mismo. Se llama vyos-sflow ✨
Un solo binario.
Corre en el propio router,
o lo pones en cualquier máquina Linux y que todos los equipos manden su sFlow ahí.

## 📦 Instalación en tres pasos

Primero, activa sFlow en VyOS (sáltate esto si ya envía a 127.0.0.1:6343):

```
configure
set system sflow interface eth0
set system sflow server 127.0.0.1 port 6343
commit
save
exit
```

¿No es VyOS? La misma idea: apunta la exportación sFlow de ese equipo a esta máquina, puerto 6343, y sigue abajo.

Después descárgalo y ejecútalo:

```
curl -fLO https://github.com/githubflyideas/vyos-sflow/releases/download/v1.3.3/vyos-sflow-1.3.3-linux-amd64.tar.gz
tar -xzf vyos-sflow-1.3.3-linux-amd64.tar.gz
cd vyos-sflow-1.3.3-linux-amd64 && ./vyos-sflow serve
```

Abre `http://ROUTER_IP:8066` en el navegador, entra con admin / tryvyossflow66 y cambia la contraseña.
Eso es todo. Los gráficos empiezan a llenarse.

![Página de resumen](assets/img/screenshots/vyos/web-overview.png)

## 👀 Lo que obtienes

✅ El ancho de banda actual, y la curva de las últimas 48 horas

✅ Top talkers: qué cliente y qué servidor usan más tráfico, en ranking

✅ El tráfico de interfaz desde los contadores del propio equipo, junto a la estimación por muestreo, para ver si los números cuadran

✅ Los registros de flujo detrás de los gráficos: pincha un pico y descubre quién fue

✅ Si un gráfico está vacío, la página Sources te dice por qué. Sin adivinar

![Top talkers](assets/img/screenshots/vyos/web-top.png)

## 🖥️ ¿Sin navegador? Usa la terminal

Entra por SSH al router y ejecuta un comando:

```
./vyos-sflow tui
```

La misma imagen, en tu terminal. Pulsa q para salir.
No se instala nada, ideal para un vistazo rápido.

Extras pequeños: la interfaz web habla 15 idiomas y tiene cinco temas, de claro a oscuro.

## ⚠️ Los límites, con honestidad

- Solo sFlow. Nada de NetFlow ni IPFIX
- El historial es de 48 horas. Responde «qué está pasando» y «qué acaba de pasar», no es para informes a largo plazo
- Solo Linux amd64. Cubre VyOS y servidores Linux normales, pero no hay versión para Windows ni macOS

Si esos tres puntos te valen, hace bien su trabajo.

## 🔗 Consíguelo

Gratis y de código abierto, Apache-2.0.
Busca vyos-sflow en GitHub, o usa el enlace de descarga de este sitio.
Si no te gusta, borra la carpeta y listo: el router queda intacto.

Si VyOS es la salida de tu oficina o la puerta de un cibercafé, pruébalo.
Esa sensación de «¿a dónde se fue mi tráfico?» desaparece.

¿Qué usas tú para vigilar el tráfico de salida? Cuéntame abajo 👇

#VyOS #sFlow #opensource #redes #sysadmin
