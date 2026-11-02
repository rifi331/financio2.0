package web

import (
	"html/template"
	"net/http"

	"github.com/rifi331/financio/internal/store"
)

type loginData struct{ Wrong bool }

type dashRow struct {
	ID                                             int64
	Date, UserName, Item, Category, Amount, Source string
	Income                                         bool
}

type dashData struct {
	Month                       string
	Categories                  []string
	Category, Direction, Totals string
	Rows                        []dashRow
	Count                       int
}

type logsData struct{ Lines []string }

var pageTmpl = template.Must(template.New("page").Parse(`<!DOCTYPE html>
<html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>Financio</title><style>
body{font-family:system-ui,sans-serif;margin:0;background:#f6f7f9;color:#1c2733}
header{background:#1c2733;color:#fff;padding:.7rem 1.2rem;display:flex;gap:1.2rem;align-items:center}
header a{color:#cfe3ff;text-decoration:none;font-weight:600}
header .sp{flex:1}
main{max-width:960px;margin:1.2rem auto;padding:0 1rem}
table{border-collapse:collapse;width:100%;background:#fff;border-radius:8px;overflow:hidden;box-shadow:0 1px 3px rgba(0,0,0,.12)}
th,td{padding:.45rem .7rem;text-align:left;border-bottom:1px solid #edf0f3;font-size:.92rem}
th{background:#eef2f6;font-size:.8rem;text-transform:uppercase;letter-spacing:.04em}
tr:last-child td{border-bottom:none}
tr.row{cursor:pointer}tr.row:hover{background:#eef3f8}
.inc{color:#0a7d33}.exp{color:#b3261e}
form.f{display:flex;gap:.6rem;flex-wrap:wrap;margin-bottom:1rem;align-items:end;background:#fff;padding:.8rem;border-radius:8px;box-shadow:0 1px 3px rgba(0,0,0,.12)}
label{font-size:.75rem;display:block;color:#5a6b7b;margin-bottom:.15rem}
select,input[type=month]{padding:.35rem .5rem;border:1px solid #cdd6df;border-radius:6px;background:#fff}
button{padding:.4rem .9rem;border:0;border-radius:6px;background:#1c2733;color:#fff;cursor:pointer}
.tot{margin:.9rem 0;font-size:1.05rem}.tot b{font-variant-numeric:tabular-nums}
pre{background:#101620;color:#cfe3d0;padding:1rem;border-radius:8px;overflow:auto;font-size:.8rem;max-height:80vh}
.meta{color:#5a6b7b;font-size:.85rem;margin:.6rem 0}
.center{max-width:340px;margin:12vh auto;background:#fff;padding:2rem;border-radius:10px;box-shadow:0 2px 8px rgba(0,0,0,.15);text-align:center}
.center input{width:100%;margin:.8rem 0;padding:.55rem;border:1px solid #cdd6df;border-radius:6px;box-sizing:border-box}
.err{color:#b3261e;font-size:.9rem}
</style></head><body>
<header><a href="/">💳 Financio</a><a href="/logs">Logs</a><span class="sp"></span><a href="/logout">Log out</a></header>
<main>{{template "content" .}}</main></body></html>`))

var loginTmpl = template.Must(template.Must(pageTmpl.Clone()).Parse(`{{define "content"}}
<div class="center"><h3>Financio dashboard</h3>
{{if .Wrong}}<p class="err">Wrong token, try again.</p>{{end}}
<form method="post"><label>Dashboard token</label>
<input type="password" name="token" autofocus><button>Sign in</button></form></div>
{{end}}`))

var dashTmpl = template.Must(template.Must(pageTmpl.Clone()).Parse(`{{define "content"}}
<form class="f" method="get" action="/">
<div><label>Month</label><input type="month" name="month" value="{{.Month}}"></div>
<div><label>Category</label><select name="category"><option value="">All</option>
{{range .Categories}}<option value="{{.}}" {{if eq . $.Category}}selected{{end}}>{{.}}</option>{{end}}
</select></div>
<div><label>Direction</label><select name="direction">
<option value="">All</option>
<option value="expense" {{if eq .Direction "expense"}}selected{{end}}>Expense</option>
<option value="income" {{if eq .Direction "income"}}selected{{end}}>Income</option>
</select></div>
<button>Apply</button></form>
<p class="tot">Net for {{.Month}} ({{.Count}} txns): <b>{{.Totals}}</b></p>
<table><tr><th>Date</th><th>User</th><th>Item</th><th>Category</th><th>Amount</th><th>Src</th></tr>
{{range .Rows}}<tr class="row" onclick="location.href='/txn/{{.ID}}'"><td>{{.Date}}</td><td>{{.UserName}}</td><td>{{.Item}}</td><td>{{.Category}}</td>
<td class="{{if .Income}}inc{{else}}exp{{end}}">{{if .Income}}+{{end}}{{.Amount}}</td><td>{{.Source}}</td></tr>
{{else}}<tr><td colspan="6">No transactions for this filter.</td></tr>{{end}}
</table>
{{end}}`))

var logsTmpl = template.Must(template.Must(pageTmpl.Clone()).Parse(`{{define "content"}}
<p class="meta">Last {{len .Lines}} log lines — refreshes every 5s. <a href="/logs">reload</a></p>
<pre id="logs">{{range .Lines}}{{.}}
{{end}}</pre>
<script>setTimeout(async()=>{try{const r=await fetch('/api/logs');const j=await r.json();
document.getElementById('logs').textContent=j.lines.join('\n');}catch(e){}location.reload},5000)</script>
{{end}}`))

func renderPage(w http.ResponseWriter, t *template.Template, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := t.Execute(w, data); err != nil {
		http.Error(w, "template error: "+err.Error(), http.StatusInternalServerError)
	}
}

type detailData struct {
	Txn     store.DashboardDetail
	Details []detailLine
	RawJSON string
}

var detailTmpl = template.Must(template.Must(pageTmpl.Clone()).Parse(`{{define "content"}}
<p class="meta"><a href="/">&larr; back to dashboard</a></p>
<table>
<tr><th>ID</th><td>{{.Txn.ID}}</td></tr>
<tr><th>Item</th><td>{{.Txn.Item}}</td></tr>
<tr><th>Category</th><td>{{.Txn.CategoryName}}</td></tr>
<tr><th>Amount</th><td class="{{if eq .Txn.Direction "income"}}inc{{else}}exp{{end}}">{{.Txn.CurrencyCode}} {{.Txn.TotalAmount}}</td></tr>
<tr><th>Direction</th><td>{{.Txn.Direction}}</td></tr>
<tr><th>Date</th><td>{{.Txn.EntryDate.Format "2006-01-02"}}</td></tr>
<tr><th>User</th><td>{{.Txn.UserName}}</td></tr>
<tr><th>Source</th><td>{{.Txn.Source}}</td></tr>
<tr><th>Created</th><td>{{.Txn.CreatedAt.Format "2006-01-02 15:04"}}</td></tr>
</table>

{{if .Details}}
<h3 style="margin:1rem 0 .4rem">Line items</h3>
<table><tr><th>Item</th><th>Amount</th><th>Qty</th><th>Location</th><th>Note</th><th>Date</th><th>Type</th></tr>
{{range .Details}}<tr>
<td>{{.Item}}</td><td>{{.Amount}}</td><td>{{.Qty}}</td><td>{{.Location}}</td>
<td>{{.Note}}</td><td>{{.Date}}</td><td>{{.Type}}</td></tr>
{{else}}<tr><td colspan="7">No line items.</td></tr>{{end}}
</table>
{{end}}

{{if .RawJSON}}
<details style="margin-top:1rem"><summary style="cursor:pointer;color:#5a6b7b">Raw JSON (description)</summary>
<pre>{{.RawJSON}}</pre></details>
{{end}}

{{if .Txn.RawMessage}}
<details style="margin-top:.6rem"><summary style="cursor:pointer;color:#5a6b7b">Original message</summary>
<pre>{{.Txn.RawMessage}}</pre></details>
{{end}}
{{end}}`))
