# tiempo-cli

Consulta el tiempo, el polen y la calidad del aire desde la terminal. Está escrito en Go y funciona sin configuración: por defecto consulta el centro de Totana (Murcia).

## Qué muestra

- **Tiempo actual y previsión a 7 días:** temperatura, lluvia, viento e índice UV. El resumen de hoy también muestra la sensación térmica, la humedad y las horas de salida y puesta del sol.
- **Calidad del aire:** índice europeo de calidad del aire (EAQI), partículas PM2.5 y PM10, polvo, ozono y dióxido de nitrógeno.
- **Polen y partículas:** máximos diarios de hasta cinco días, según disponibilidad. Incluye un calendario estacional para pólenes sin pronóstico diario, como el ciprés.
- **Moho:** índice de 0 a 100 que estima si el tiempo favorece el moho. No mide esporas.

## Empezar

Necesitas Go 1.23 o posterior y conexión a internet. No hay dependencias externas de Go. Desde la carpeta del proyecto:

```sh
go run .
```

Para compilar el ejecutable `tiempo` y usarlo sin recompilar cada vez:

```sh
go build -o tiempo .
./tiempo
```

Si prefieres una salida sin colores, ejecuta `NO_COLOR=1 go run .`.

## Configuración local

En Linux, la app lee `~/.config/tiempo/config.json`. Si tienes definida la variable `XDG_CONFIG_HOME`, usa `$XDG_CONFIG_HOME/tiempo/config.json`. El archivo es opcional: si no existe, se usan las coordenadas públicas del centro de Totana y no se muestra la columna «Tu perfil».

Usa [config.example.json](config.example.json) como plantilla para tu archivo local:

- `latitude` y `longitude`: coordenadas en grados decimales. Si creas el archivo, indica ambas.
- `profile`: notas opcionales por nombre de polen, tal como aparece en la tabla. Por ejemplo, `"profile": {"Olivo": "nota de ejemplo"}`. Las notas no cambian los pronósticos ni sus umbrales.

Las coordenadas se envían a Open-Meteo para hacer la consulta. Las notas solo se leen y muestran localmente. Si incluyes datos sensibles, mantén el archivo fuera de Git, dale permisos `600` y evita compartir capturas que los muestren.

## Fuentes y límites

Los pronósticos provienen de [Open-Meteo](https://open-meteo.com/). Los datos de polen y calidad del aire se basan en [CAMS](https://open-meteo.com/en/docs/air-quality-api) y su cobertura no siempre abarca los siete días. La tabla usa `—` para los valores diarios que no están disponibles. Si falla la consulta de calidad del aire, la app avisa y sigue mostrando el tiempo.

Puedes consultar otras coordenadas, pero **el encabezado sigue diciendo «Totana (Murcia)»**, la zona horaria sigue siendo `Europe/Madrid` y los avisos estacionales siguen el calendario de REAREMUR para Murcia.

## Pruebas

```sh
go test ./...
```
