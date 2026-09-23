package webadmin

import (
	"html/template"
	"time"

	"github.com/Masterminds/sprig/v3"
)

var tmplDashboard = template.Must(template.New("dashboard").Funcs(sprig.FuncMap()).Funcs(template.FuncMap{
	"estadoNombre": EstadoNombre,
	"fmtTime": func(t time.Time) string {
		if t.IsZero() {
			return "—"
		}
		return t.Format("2006-01-02 15:04:05")
	},
}).Parse(`<!DOCTYPE html>
<html lang="es">
<head>
    <meta charset="utf-8">
    <title>Relay SMTP — Dashboard</title>
    <style>
        body { font-family: system-ui, sans-serif; margin: 0; background: #f5f5f5; }
        header { background: #222; color: #fff; padding: 12px 24px; display: flex; gap: 16px; align-items: center; }
        header a { color: #ddd; text-decoration: none; }
        header a:hover { color: #fff; }
        header input { padding: 4px 8px; border-radius: 4px; border: 1px solid #555; background: #333; color: #fff; }
        main { padding: 24px; max-width: 1100px; margin: 0 auto; }
        .cards { display: grid; grid-template-columns: repeat(4, 1fr); gap: 16px; margin-bottom: 24px; }
        .card { background: #fff; padding: 16px; border-radius: 6px; box-shadow: 0 1px 3px rgba(0,0,0,.1); }
        .card h3 { margin: 0 0 8px; font-size: 13px; color: #666; text-transform: uppercase; letter-spacing: .5px; }
        .card .num { font-size: 32px; font-weight: 600; }
        .card.pendiente .num { color: #d97706; }
        .card.enviado .num { color: #16a34a; }
        .card.fallido .num { color: #dc2626; }
        .card.enviando .num { color: #2563eb; }
        .stats { background: #fff; padding: 16px; border-radius: 6px; margin-bottom: 24px; display: flex; gap: 32px; }
        .stats div { font-size: 14px; color: #555; }
        .stats strong { color: #222; }
        table { width: 100%; border-collapse: collapse; background: #fff; border-radius: 6px; overflow: hidden; box-shadow: 0 1px 3px rgba(0,0,0,.1); }
        th, td { padding: 10px 12px; text-align: left; border-bottom: 1px solid #eee; font-size: 14px; }
        th { background: #fafafa; font-weight: 600; color: #444; }
        tr:last-child td { border-bottom: none; }
        tr:hover { background: #fafafa; }
        .badge { display: inline-block; padding: 2px 8px; border-radius: 12px; font-size: 12px; font-weight: 500; }
        .badge.pendiente { background: #fef3c7; color: #92400e; }
        .badge.enviado { background: #dcfce7; color: #166534; }
        .badge.fallido { background: #fee2e2; color: #991b1b; }
        .badge.enviando { background: #dbeafe; color: #1e40af; }
        a.btn { color: #2563eb; text-decoration: none; }
        a.btn:hover { text-decoration: underline; }
        .actions { margin: 16px 0; display: flex; gap: 8px; }
        form.inline { display: inline; }
        button { padding: 6px 12px; border-radius: 4px; border: 1px solid #ccc; background: #fff; cursor: pointer; font-size: 14px; }
        button:hover { background: #f0f0f0; }
        button.primary { background: #2563eb; color: #fff; border-color: #2563eb; }
        button.primary:hover { background: #1d4ed8; }
        button.danger { background: #dc2626; color: #fff; border-color: #dc2626; }
        button.danger:hover { background: #b91c1c; }
    </style>
</head>
<body>
<header>
    <strong>Relay SMTP</strong>
    <a href="/">Dashboard</a>
    <a href="/correos">Todos</a>
    <a href="/correos?estado=0">Pendientes</a>
    <a href="/correos?estado=2">Enviados</a>
    <a href="/correos?estado=3">Fallidos</a>
    <form method="get" action="/buscar" style="margin-left:auto">
        <input name="q" placeholder="Buscar..." />
    </form>
</header>
<main>
    <div class="cards">
        <div class="card pendiente"><h3>Pendientes</h3><div class="num">{{index .Stats.PorEstado 0}}</div></div>
        <div class="card enviando"><h3>Enviando</h3><div class="num">{{index .Stats.PorEstado 1}}</div></div>
        <div class="card enviado"><h3>Enviados</h3><div class="num">{{index .Stats.PorEstado 2}}</div></div>
        <div class="card fallido"><h3>Fallidos</h3><div class="num">{{index .Stats.PorEstado 3}}</div></div>
    </div>

    <div class="stats">
        <div>Total: <strong>{{.Stats.Total}}</strong></div>
        <div>Enviados hoy: <strong>{{.Stats.EnviadosHoy}}</strong></div>
        <div>Enviados 7d: <strong>{{.Stats.EnviadosSemana}}</strong></div>
        <div>Tasa de éxito: <strong>{{printf "%.1f%%" (mulf .Stats.TasaExito 100)}}</strong></div>
    </div>

    <div class="actions">
        <form method="post" action="/accion/reintentar-fallidos" onsubmit="return confirm('¿Reintentar TODOS los fallidos?');">
            <button class="primary">Reintentar todos los fallidos</button>
        </form>
    </div>

    <h2>Últimos 10 correos</h2>
    <table>
        <thead>
            <tr><th>ID</th><th>De</th><th>Para</th><th>Asunto</th><th>Estado</th><th>Intentos</th><th>Creado</th><th></th></tr>
        </thead>
        <tbody>
        {{range .Ultimos}}
            <tr>
                <td>{{.ID}}</td>
                <td>{{.Remitente}}</td>
                <td>{{.Destinatarios}}</td>
                <td>{{.Asunto}}</td>
                <td><span class="badge {{estadoNombre .Estado}}">{{estadoNombre .Estado}}</span></td>
                <td>{{.Intentos}}</td>
                <td>{{fmtTime .Creado}}</td>
                <td><a class="btn" href="/correo/{{.ID}}">Ver</a></td>
            </tr>
        {{else}}
            <tr><td colspan="8" style="text-align:center;color:#999">No hay correos</td></tr>
        {{end}}
        </tbody>
    </table>
</main>
</body>
</html>`))

var tmplListado = template.Must(template.New("listado").Funcs(template.FuncMap{
	"estadoNombre": EstadoNombre,
	"fmtTime": func(t time.Time) string {
		if t.IsZero() {
			return "—"
		}
		return t.Format("2006-01-02 15:04:05")
	},
}).Parse(`<!DOCTYPE html>
<html lang="es">
<head>
    <meta charset="utf-8">
    <title>Relay SMTP — Correos</title>
    <style>
        body { font-family: system-ui, sans-serif; margin: 0; background: #f5f5f5; }
        header { background: #222; color: #fff; padding: 12px 24px; display: flex; gap: 16px; align-items: center; }
        header a { color: #ddd; text-decoration: none; }
        header a:hover { color: #fff; }
        header input { padding: 4px 8px; border-radius: 4px; border: 1px solid #555; background: #333; color: #fff; }
        main { padding: 24px; max-width: 1200px; margin: 0 auto; }
        table { width: 100%; border-collapse: collapse; background: #fff; border-radius: 6px; overflow: hidden; box-shadow: 0 1px 3px rgba(0,0,0,.1); }
        th, td { padding: 10px 12px; text-align: left; border-bottom: 1px solid #eee; font-size: 14px; }
        th { background: #fafafa; font-weight: 600; color: #444; }
        tr:hover { background: #fafafa; }
        .badge { display: inline-block; padding: 2px 8px; border-radius: 12px; font-size: 12px; font-weight: 500; }
        .badge.pendiente { background: #fef3c7; color: #92400e; }
        .badge.enviado { background: #dcfce7; color: #166534; }
        .badge.fallido { background: #fee2e2; color: #991b1b; }
        .badge.enviando { background: #dbeafe; color: #1e40af; }
        .pager { margin-top: 16px; display: flex; gap: 8px; }
        .pager a, .pager span { padding: 6px 12px; border-radius: 4px; border: 1px solid #ccc; background: #fff; text-decoration: none; color: #333; font-size: 14px; }
        .pager span.disabled { color: #bbb; }
        .pager a:hover { background: #f0f0f0; }
        .err { color: #991b1b; font-size: 12px; }
        .truncate { max-width: 240px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
    </style>
</head>
<body>
<header>
    <strong>Relay SMTP</strong>
    <a href="/">Dashboard</a>
    <a href="/correos">Todos</a>
    <a href="/correos?estado=0">Pendientes</a>
    <a href="/correos?estado=2">Enviados</a>
    <a href="/correos?estado=3">Fallidos</a>
    <form method="get" action="/buscar" style="margin-left:auto">
        <input name="q" placeholder="Buscar..." />
    </form>
</header>
<main>
    <h1>
        Correos
        {{if .Query}} — búsqueda: "{{.Query}}"{{end}}
        {{if .Estado}} — estado: {{.Estado}}{{end}}
    </h1>
    <table>
        <thead>
            <tr><th>ID</th><th>De</th><th>Para</th><th>Asunto</th><th>Estado</th><th>Intentos</th><th>Próximo</th><th>Último error</th><th></th></tr>
        </thead>
        <tbody>
        {{range .Correos}}
            <tr>
                <td>{{.ID}}</td>
                <td class="truncate">{{.Remitente}}</td>
                <td class="truncate">{{.Destinatarios}}</td>
                <td class="truncate">{{.Asunto}}</td>
                <td><span class="badge {{estadoNombre .Estado}}">{{estadoNombre .Estado}}</span></td>
                <td>{{.Intentos}}</td>
                <td>{{fmtTime .ProximoIntento}}</td>
                <td class="err truncate">{{.UltimoError}}</td>
                <td><a href="/correo/{{.ID}}">Ver</a></td>
            </tr>
        {{else}}
            <tr><td colspan="9" style="text-align:center;color:#999">No hay correos</td></tr>
        {{end}}
        </tbody>
    </table>

    <div class="pager">
        {{if gt .Page 1}}
            <a href="?estado={{.Estado}}&page={{.PrevPage}}">← Anterior</a>
        {{else}}
            <span class="disabled">← Anterior</span>
        {{end}}
        <span>Página {{.Page}}</span>
        {{if eq (len .Correos) 50}}
            <a href="?estado={{.Estado}}&page={{.NextPage}}">Siguiente →</a>
        {{else}}
            <span class="disabled">Siguiente →</span>
        {{end}}
    </div>
</main>
</body>
</html>`))

var tmplDetalle = template.Must(template.New("detalle").Funcs(template.FuncMap{
	"estadoNombre": EstadoNombre,
	"fmtTime": func(t time.Time) string {
		if t.IsZero() {
			return "—"
		}
		return t.Format("2006-01-02 15:04:05")
	},
}).Parse(`<!DOCTYPE html>
<html lang="es">
<head>
    <meta charset="utf-8">
    <title>Relay SMTP — Correo {{.Correo.ID}}</title>
    <style>
        body { font-family: system-ui, sans-serif; margin: 0; background: #f5f5f5; }
        header { background: #222; color: #fff; padding: 12px 24px; display: flex; gap: 16px; align-items: center; }
        header a { color: #ddd; text-decoration: none; }
        header a:hover { color: #fff; }
        main { padding: 24px; max-width: 1000px; margin: 0 auto; }
        .panel { background: #fff; padding: 20px; border-radius: 6px; box-shadow: 0 1px 3px rgba(0,0,0,.1); margin-bottom: 16px; }
        .panel h2 { margin-top: 0; }
        dl { display: grid; grid-template-columns: 180px 1fr; gap: 8px 16px; margin: 0; }
        dt { color: #666; font-size: 14px; }
        dd { margin: 0; font-size: 14px; }
        .badge { display: inline-block; padding: 2px 8px; border-radius: 12px; font-size: 12px; font-weight: 500; }
        .badge.pendiente { background: #fef3c7; color: #92400e; }
        .badge.enviado { background: #dcfce7; color: #166534; }
        .badge.fallido { background: #fee2e2; color: #991b1b; }
        .badge.enviando { background: #dbeafe; color: #1e40af; }
        pre { background: #1e1e1e; color: #ddd; padding: 16px; border-radius: 4px; overflow-x: auto; font-size: 13px; line-height: 1.4; max-height: 500px; }
        .actions { display: flex; gap: 8px; margin-top: 16px; }
        button { padding: 8px 16px; border-radius: 4px; border: 1px solid #ccc; background: #fff; cursor: pointer; font-size: 14px; }
        button:hover { background: #f0f0f0; }
        button.primary { background: #2563eb; color: #fff; border-color: #2563eb; }
        button.primary:hover { background: #1d4ed8; }
        button.danger { background: #dc2626; color: #fff; border-color: #dc2626; }
        button.danger:hover { background: #b91c1c; }
        .err { color: #991b1b; }
    </style>
</head>
<body>
<header>
    <strong>Relay SMTP</strong>
    <a href="/">Dashboard</a>
    <a href="/correos">Todos</a>
    <a href="/correos?estado=0">Pendientes</a>
    <a href="/correos?estado=2">Enviados</a>
    <a href="/correos?estado=3">Fallidos</a>
</header>
<main>
    <h1>Correo #{{.Correo.ID}}</h1>

    <div class="panel">
        <dl>
            <dt>Estado</dt>
            <dd><span class="badge {{estadoNombre .Correo.Estado}}">{{estadoNombre .Correo.Estado}}</span></dd>
            <dt>Remitente</dt>
            <dd>{{.Correo.Remitente}}</dd>
            <dt>Destinatarios</dt>
            <dd>{{.Correo.Destinatarios}}</dd>
            <dt>Asunto</dt>
            <dd>{{.Correo.Asunto}}</dd>
            <dt>Intentos</dt>
            <dd>{{.Correo.Intentos}}</dd>
            <dt>Próximo intento</dt>
            <dd>{{fmtTime .Correo.ProximoIntento}}</dd>
            <dt>Creado</dt>
            <dd>{{fmtTime .Correo.Creado}}</dd>
            <dt>Enviado</dt>
            <dd>{{fmtTime .Correo.Enviado.Time}}</dd>
            {{if .Correo.UltimoError}}
            <dt>Último error</dt>
            <dd class="err">{{.Correo.UltimoError}}</dd>
            {{end}}
        </dl>

        <div class="actions">
            <form method="post" action="/accion/forzar">
                <input type="hidden" name="id" value="{{.Correo.ID}}">
                <button class="primary">Forzar reintento</button>
            </form>
            <form method="post" action="/accion/eliminar" onsubmit="return confirm('¿Eliminar el correo #{{.Correo.ID}}?');">
                <input type="hidden" name="id" value="{{.Correo.ID}}">
                <button class="danger">Eliminar</button>
            </form>
        </div>
    </div>

    <div class="panel">
        <h2>Mensaje crudo (.eml)</h2>
        <pre>{{.Raw}}</pre>
    </div>
</main>
</body>
</html>`))
