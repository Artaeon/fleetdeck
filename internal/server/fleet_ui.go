package server

const fleetHTML = `<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>FleetDeck · Server overview</title>
<link rel="stylesheet" href="/static/style.css"><style>
main{max-width:1280px;margin:auto;padding:32px}header,.tools{display:flex;gap:16px;align-items:center;flex-wrap:wrap;margin-bottom:24px}header h1{flex:1}p{color:var(--text-secondary);margin:12px 0}.notice{padding:16px;border:1px solid var(--border);margin:24px 0}.table-scroll{overflow:auto}table{width:100%;border-collapse:collapse}th,td{text-align:left;padding:14px;border-bottom:1px solid var(--border)}button,select{font:inherit;color:var(--text);background:var(--bg-card);border:1px solid var(--border);padding:10px;cursor:pointer}button:disabled{opacity:.5;cursor:wait}h2{margin-top:24px}.unknown{color:var(--yellow)}.attention{color:var(--red)}.ok{color:var(--green)}.muted{color:var(--text-secondary);font-size:.85em}#detail{border:1px solid var(--border);padding:20px;margin-top:24px}a{color:var(--accent)}@media(max-width:640px){main{padding:16px}th,td{padding:10px}}
</style><script src="/static/app.js" defer></script></head><body><main>
<header><h1>FleetDeck <span class="muted">/ servers</span></h1><form action="/logout" method="post"><button>Sign out</button></form></header>
<p>Your servers, visible only on this device. Read-only access follows your assigned groups.</p>
<div class="notice">This overview does not deploy, move or restart applications. A server group does not provide automatic failover.</div>
<div class="tools"><label for="group">Server group</label><select id="group"><option value="">All permitted groups</option></select><button id="refresh">Refresh</button><span id="updated" class="muted" role="status"></span></div>
<p id="message" role="status">Loading permitted servers…</p>
<div class="table-scroll"><table><thead><tr><th>Server</th><th>Observation</th><th>Projects</th><th>Monitoring</th><th>Security</th><th>Access</th></tr></thead><tbody id="servers"></tbody></table></div>
<section id="detail" hidden aria-live="polite"></section>
<p class="muted">Observations can be delayed or unavailable. Registry status is not a live container check; a backup record does not prove recovery.</p>
</main></body></html>`

const fleetJS = `(() => {
'use strict';
const $ = id => document.getElementById(id);
let rows = [], loading = false, generation = 0;
const text = (tag,value,parent,cls) => {const e=document.createElement(tag);e.textContent=value;if(cls)e.className=cls;if(parent)parent.appendChild(e);return e;};
const date = value => value ? new Date(value).toLocaleString() : 'Unknown';
async function api(path) {const r=await fetch(path,{credentials:'same-origin',cache:'no-store'});if(r.status===401){location.assign('/login');throw Error('Sign in again');}if(!r.ok)throw Error('Current state unavailable. Access may have changed.');return r.json();}
function hideDetails(){generation++;$('detail').hidden=true;$('detail').replaceChildren();}
function render(){
 $('servers').replaceChildren();
 for(const n of rows.filter(n=>!$('group').value||n.group===$('group').value)){
  const tr=document.createElement('tr');$('servers').appendChild(tr);
  const name=text('td','',tr);text('strong',n.label,name);text('div',n.group||'Ungrouped',name,'muted');
  const state=text('td',n.status==='observed'?'Received':'Unknown',tr,n.status==='observed'?'':'unknown');if(n.observed_at)text('div',date(n.observed_at),state,'muted');if(n.version)text('div','FleetDeck '+n.version,state,'muted');
  text('td',n.status==='observed'?String(n.projects):'—',tr);
  const m=n.monitoring;const mc=text('td',m?m.status:'unknown',tr,m?m.status:'unknown');if(m)text('div',m.healthy+' healthy · '+m.unhealthy+' failing · '+m.unknown+' unknown',mc,'muted');
  const s=n.security;const sc=text('td',s?s.status:'unknown',tr,s?s.status:'unknown');if(s){text('div',s.critical+' critical · '+s.high+' high · '+s.runtime_findings+' runtime · '+s.unknown+' unknown',sc,'muted');text('div',date(s.checked_at),sc,'muted');}
  const access=text('td','',tr);if(n.details){const b=text('button','Project details',access);b.addEventListener('click',()=>details(n));}else{text('span','Summary only',access,'muted');}
 }
}
async function details(n){hideDetails();const current=generation;const box=$('detail');box.hidden=false;text('p','Loading…',box);try{const s=await api('/api/fleet/'+encodeURIComponent(n.id));if(current!==generation)return;box.replaceChildren();text('h2',n.label+' / projects',box);text('p','Observed '+date(s.generated_at)+' · FleetDeck '+s.version,box);const table=document.createElement('table');box.appendChild(table);const head=document.createElement('tr');table.appendChild(head);for(const label of ['Project','Recorded state','Last backup record'])text('th',label,head);for(const p of s.projects){const tr=document.createElement('tr');table.appendChild(tr);text('td',p.name,tr);text('td',p.registry_status,tr);text('td',date(p.last_backup),tr);}if(!s.projects.length)text('p','No registered projects.',box);}catch(e){if(current===generation){box.replaceChildren();text('p',e.message,box,'unknown');}}}
async function refresh(){if(loading)return;loading=true;$('refresh').disabled=true;hideDetails();rows=[];render();$('message').textContent='Refreshing permitted servers…';try{const result=await api('/api/fleet');rows=result.servers;const selected=$('group').value;$('group').replaceChildren();const all=text('option','All permitted groups',$('group'));all.value='';for(const group of [...new Set(rows.map(n=>n.group).filter(Boolean))].sort()){const option=text('option',group,$('group'));option.value=group;}$('group').value=[...$('group').options].some(o=>o.value===selected)?selected:'';render();$('message').textContent=rows.length?rows.length+' permitted servers':'No servers are assigned to your groups. Ask the local operator to review enrollment and access.';$('updated').textContent='Refreshed '+new Date().toLocaleTimeString();}catch(e){rows=[];render();$('message').textContent=e.message;$('updated').textContent='';}finally{loading=false;$('refresh').disabled=false;}}
$('group').addEventListener('change',()=>{hideDetails();render();});$('refresh').addEventListener('click',refresh);
refresh();setInterval(()=>{if(!document.hidden)refresh();},30000);
})();`
