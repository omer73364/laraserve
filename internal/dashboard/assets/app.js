'use strict';
const $ = id => document.getElementById(id);
let csrf = '', projects = [], fetching = false, logRequest, searchTimer;
const localBusy = new Set();
const escape = value => String(value ?? '').replace(/[&<>"']/g, c => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
const bytes = n => n >= 1073741824 ? (n / 1073741824).toFixed(1) + ' GB' : (n / 1048576).toFixed(0) + ' MB';
function message(text) { $('notice').textContent = text; $('notice').hidden = !text; }
async function request(path, options = {}) {
 const response = await fetch(path, {cache:'no-store', credentials:'same-origin', ...options});
 if (!response.ok) { if (response.status === 401) { $('locked').hidden = false; $('workspace').hidden = true; } throw new Error((await response.text()).trim() || 'Request failed'); }
 return response.status === 202 || response.status === 204 ? null : response.json();
}
function card(p) {
 const active = ['running','starting','stopping','unresponsive'].includes(p.status);
 const pending = p.operation?.pending || localBusy.has(p.id);
 const busy = pending ? ' disabled' : '';
 const usage = p.resources.available ? `<span>CPU ${p.resources.cpu_percent == null ? 'sampling…' : p.resources.cpu_percent.toFixed(1) + '%'}</span><span>RAM ${bytes(p.resources.rss_bytes)}</span>` : `<span>${escape(p.health)}</span>`;
 let open = '';
 try { const u = new URL(p.url); if (['http:','https:'].includes(u.protocol) && !u.username && !u.password && p.status === 'running') open = `<a class="button open" href="${escape(u.href)}" target="_blank" rel="noopener noreferrer">Open app ↗</a>`; } catch {}
 return `<article class="project-card" data-project="${p.id}"><div class="card-body"><div class="card-top"><span class="app-icon">${escape(p.name.slice(0,1).toUpperCase())}</span><div class="card-title"><h3 title="${escape(p.name)}">${escape(p.name)}</h3><div class="project-url">${escape(p.url || 'Ready when you are')}</div></div><span class="badge ${escape(p.status)}">${escape(p.status)}</span></div><div class="runtime-row"><span class="runtime"><b>PHP</b> ${escape(p.php || 'Not selected')}</span><span class="runtime">${escape(p.database || 'Database not selected')}</span></div><div class="resource" title="${escape(p.resources.note || p.health)}">${usage}</div><div class="path" title="${escape(p.source)}">${escape(p.source)}</div></div>${p.error || p.operation?.error ? `<div class="card-error">${escape(p.error || p.operation.error)}</div>` : ''}<div class="card-actions"><button class="button primary" data-action="${active?'stop':'start'}" data-id="${p.id}"${busy}>${pending ? 'Working…' : active ? '■ Stop' : '▷ Start'}</button>${active ? `<button class="button" data-action="restart" data-id="${p.id}"${busy}>↻ Restart</button>` : ''}<button class="button" data-logs="${p.id}">Logs</button>${open}</div></article>`;
}
function syncCards(shown) {
 const list = $('project-list');
 const existing = new Map([...list.children].map(node => [node.dataset.project, node]));
 for (let index = 0; index < shown.length; index++) {
  const p = shown[index], template = document.createElement('template');
  template.innerHTML = card(p);
  const next = template.content.firstElementChild;
  let node = existing.get(p.id);
  if (!node) node = next;
  else {
   const oldBody = node.querySelector('.card-body'), newBody = next.querySelector('.card-body');
   if (oldBody.innerHTML !== newBody.innerHTML) oldBody.replaceWith(newBody);
   const oldError = node.querySelector('.card-error'), newError = next.querySelector('.card-error');
   if (oldError && !newError) oldError.remove();
   else if (newError && oldError) { if (oldError.textContent !== newError.textContent) oldError.replaceWith(newError); }
   else if (newError) node.insertBefore(newError, node.querySelector('.card-actions'));
   const actions = node.querySelector('.card-actions');
   const key = el => el.dataset.action || (el.dataset.logs ? 'logs' : 'open');
   const oldActions = new Map([...actions.children].map(el => [key(el), el]));
   for (let i = 0; i < next.querySelector('.card-actions').children.length; i++) {
    const wanted = next.querySelector('.card-actions').children[i];
    let el = oldActions.get(key(wanted));
    if (el) {
     oldActions.delete(key(wanted)); el.textContent = wanted.textContent; el.className = wanted.className;
     if (el.tagName === 'BUTTON') el.disabled = wanted.disabled;
     else el.href = wanted.href;
    } else el = wanted.cloneNode(true);
    if (actions.children[i] !== el) actions.insertBefore(el, actions.children[i] || null);
   }
   for (const el of oldActions.values()) el.remove();
  }
  existing.delete(p.id);
  if (list.children[index] !== node) list.insertBefore(node, list.children[index] || null);
 }
 for (const node of existing.values()) node.remove();
}
function render() {
 const query = $('project-search').value.toLowerCase();
 const shown = projects.filter(p => `${p.name} ${p.source} ${p.url}`.toLowerCase().includes(query));
 syncCards(shown);
 $('total').textContent = $('project-count').textContent = $('nav-count').textContent = projects.length;
 $('running').textContent = projects.filter(p => p.status === 'running').length;
 const measured = projects.filter(p => p.resources.available);
 $('memory').textContent = measured.length ? bytes(measured.reduce((sum,p)=>sum+p.resources.rss_bytes,0)) : '—';
 $('empty').hidden = projects.length !== 0;
 $('no-match').hidden = !projects.length || shown.length !== 0;
 const selected = $('log-project').value;
 const signature = projects.map(p=>p.id+p.name).join('|');
 if ($('log-project').dataset.signature !== signature) {
  $('log-project').innerHTML = '<option value="">Select a project</option>' + projects.map(p=>`<option value="${p.id}">${escape(p.name)} · ${p.id.slice(0,6)}</option>`).join('');
  $('log-project').value = selected; $('log-project').dataset.signature = signature;
 }
}
async function refresh() {
 if (fetching || !csrf) return;
 fetching = true;
 try { projects = await request('/api/projects'); render(); $('updated').textContent = 'Updated ' + new Date().toLocaleTimeString([], {hour:'2-digit',minute:'2-digit',second:'2-digit'}); message(''); }
 catch (e) {message(e.message);} finally {fetching=false;}
}
async function loadLogs() {
 if (logRequest) logRequest.abort();
 const id = $('log-project').value;
 if (!id) { $('log-output').textContent='Choose a project above or select “Logs” on a project card.'; $('log-state').textContent='Select a project to inspect';return; }
 logRequest = new AbortController();
 $('log-state').textContent = 'Reading recent output…';
 try {
  const data=await request(`/api/projects/${id}/logs?`+new URLSearchParams({source:$('log-source').value,q:$('log-search').value}), {signal:logRequest.signal});
  $('log-output').textContent=data.text || ($('log-search').value ? 'No matching lines in recent output.' : 'No output in this log yet.');
  $('log-note').textContent=data.note;
  $('log-state').textContent=data.truncated?'Showing a bounded recent tail':'Recent output · '+new Date().toLocaleTimeString();
 } catch(e) {if(e.name!=='AbortError'){$('log-output').textContent=e.message;$('log-state').textContent='Could not read log';}}
}
$('project-list').addEventListener('click',async event=>{
 const logs=event.target.closest('[data-logs]');if(logs){$('log-project').value=logs.dataset.logs;loadLogs();$('log-output').scrollIntoView({behavior:'smooth',block:'center'});return;}
 const button=event.target.closest('[data-action]');if(!button)return;
 const id=button.dataset.id, action=button.dataset.action;
 localBusy.add(id);render();
 let actionError = '';
 try {await request(`/api/projects/${id}/${action}`,{method:'POST',headers:{'Content-Type':'application/json','X-CSRF-Token':csrf},body:'{}'});}
 catch(e){actionError=e.message;}finally{localBusy.delete(id);await refresh();if(actionError)message(actionError);}
});
$('refresh').addEventListener('click',refresh);
$('project-search').addEventListener('input',render);
$('log-project').addEventListener('change',loadLogs);
$('log-source').addEventListener('change',loadLogs);
$('refresh-logs').addEventListener('click',loadLogs);
$('log-search').addEventListener('input',()=>{clearTimeout(searchTimer);searchTimer=setTimeout(loadLogs,250);});
async function connect() {
 const token=location.hash.slice(1);
 history.replaceState(null,'',location.pathname);
 try {
  if (/^[a-f0-9]{64}$/.test(token)) await request('/session',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({token})});
  const session=await request('/api/session');csrf=session.csrf;
  $('locked').hidden=true;$('workspace').hidden=false;
  await refresh();
 }catch(e){$('locked').hidden=false;$('workspace').hidden=true;}
}
window.addEventListener('hashchange',()=>{if(/^[a-f0-9]{64}$/.test(location.hash.slice(1)))connect();});
setInterval(()=>{if(!document.hidden)refresh();},3000);
connect();
