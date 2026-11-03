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
 return `<article class="project-card" data-project="${p.id}"><div class="card-body"><div class="card-top"><span class="app-icon">${escape(p.name.slice(0,1).toUpperCase())}</span><div class="card-title"><h3 title="${escape(p.name)}">${escape(p.name)}</h3><div class="project-url">${escape(p.url || 'Ready when you are')}</div></div><span class="badge ${escape(p.status)}">${escape(p.status)}</span></div><div class="runtime-row"><span class="runtime"><b>PHP</b> ${escape(p.php || 'Not selected')}</span><span class="runtime">${escape(p.database || 'Database not selected')}</span></div><div class="resource" title="${escape(p.resources.note || p.health)}">${usage}</div><div class="path" title="${escape(p.source)}">${escape(p.source)}</div></div>${p.error || p.operation?.error ? `<div class="card-error">${escape(p.error || p.operation.error)}</div>` : ''}<div class="card-actions"><button class="button primary" data-action="${active?'stop':'start'}" data-id="${p.id}"${busy}>${pending ? 'Working…' : active ? '■ Stop' : '▷ Start'}</button>${active ? `<button class="button" data-action="restart" data-id="${p.id}"${busy}>↻ Restart</button>` : ''}<button class="button" data-logs="${p.id}">Logs</button><button class="button" data-settings="${p.id}">Settings</button>${open}</div></article>`;
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
   const key = el => el.dataset.action || (el.dataset.logs ? 'logs' : el.dataset.settings ? 'settings' : 'open');
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
 const settings=event.target.closest('[data-settings]');if(settings){openTools(settings.dataset.settings);return;}
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
  await refresh();await loadCatalog();
 }catch(e){$('locked').hidden=false;$('workspace').hidden=true;}
}
window.addEventListener('hashchange',()=>{if(/^[a-f0-9]{64}$/.test(location.hash.slice(1)))connect();});
setInterval(()=>{if(!document.hidden)refresh();},3000);
let toolProject = '', configDocument, environmentDocument, catalog = {php:[],mysql:[]}, currentTask = '', taskTimer;
const changedVariables = new Map();
const phpFields=['memory_limit','max_execution_time','upload_max_filesize','post_max_size','date.timezone','display_errors'];
function post(path, body) {return request(path,{method:'POST',headers:{'Content-Type':'application/json','X-CSRF-Token':csrf},body:JSON.stringify(body)});}
async function loadCatalog(){catalog=await request('/api/catalog');for(const kind of ['php','mysql'])$(kind+'-versions').innerHTML=catalog[kind].map(version=>`<option value="${escape(version)}"></option>`).join('');runtimeVersions();const previous=sessionStorage.getItem('laraserve-task');if(previous&&/^[a-f0-9]{12}$/.test(previous)&&!currentTask)watchTask(previous);}
function toolNotice(text){$('tools-notice').textContent=text;}
function showTab(name){for(const tab of ['settings','environment','actions'])$(tab==='actions'?'action-form':tab+'-form').hidden=tab!==name;document.querySelectorAll('[data-tool-tab]').forEach(button=>button.classList.toggle('selected',button.dataset.toolTab===name));}
async function openTools(id){
 toolProject=id;configDocument=undefined;environmentDocument=undefined;const p=projects.find(p=>p.id===id);$('tools-title').textContent=p?.name||'Project';$('project-tools').hidden=false;toolNotice('Loading settings…');showTab('settings');
 try{
  const loaded=await request(`/api/projects/${id}/settings`);if(toolProject!==id)return;configDocument=loaded;const c=configDocument.config, f=$('settings-form').elements;
  for(const key of ['domain','php','mysql','php_ini'])f[key].value=c[key]||'';
  f.port.value=c.port??'';f.database.value=c.database||'auto';f.https.value=c.https==null?'':String(c.https);f.phpmyadmin.value=c.phpmyadmin==null?'':String(c.phpmyadmin);
  for(const key of phpFields)f[key].value=c.php_settings?.[key]||'';
  toolNotice('');$('project-tools').scrollIntoView({behavior:'smooth',block:'start'});
 }catch(e){toolNotice(e.message);}
}
async function loadEnvironment(){
 const id=toolProject;environmentDocument=undefined;toolNotice('Loading masked environment…');changedVariables.clear();$('env-new-key').value='';$('env-new-value').value='';
 try{
  const loaded=await request(`/api/projects/${id}/environment`);if(toolProject!==id)return;environmentDocument=loaded;
  $('environment-fields').innerHTML=environmentDocument.variables.map(v=>`<div class="env-row"><label>${escape(v.key)} ${v.secret?'· masked':''}<input data-env-key="${escape(v.key)}" type="${v.secret?'password':'text'}" value="${escape(v.value)}" placeholder="${v.secret&&v.present?'Unchanged secret':v.present?'Empty value':'Not in .env'}" autocomplete="off"></label><small>${v.in_example?'In .env.example':'Only in .env'}${v.secret?' · example value hidden':v.in_example?'<br>Example: '+escape(v.example):''}${!v.present?' · missing from .env':''}</small></div>`).join('');
  $('env-comparison').textContent=environmentDocument.variables.filter(v=>v.in_example&&!v.present).length+' example variables missing';toolNotice('');
 }catch(e){toolNotice(e.message);}
}
document.querySelectorAll('[data-tool-tab]').forEach(button=>button.addEventListener('click',()=>{showTab(button.dataset.toolTab);if(button.dataset.toolTab==='environment')loadEnvironment();else toolNotice('');}));
$('close-tools').addEventListener('click',()=>{$('project-tools').hidden=true;});
$('settings-form').addEventListener('submit',async event=>{
 event.preventDefault();if(!configDocument)return;const f=event.target.elements,c={...configDocument.config};
 for(const key of ['domain','php','mysql','php_ini'])c[key]=f[key].value.trim();
 c.database=f.database.value;for(const key of ['https','phpmyadmin']){if(f[key].value==='')delete c[key];else c[key]=f[key].value==='true';}
 if(f.port.value==='')delete c.port;else c.port=Number(f.port.value);
 c.php_settings={...c.php_settings};for(const key of phpFields){if(f[key].value==='')delete c.php_settings[key];else c.php_settings[key]=f[key].value.trim();}
 try{configDocument=await post(`/api/projects/${toolProject}/settings`,{revision:configDocument.revision,config:c});toolNotice('Saved. Restart this project to apply the settings.');}catch(e){toolNotice(e.message);}
});
$('environment-fields').addEventListener('input',event=>{if(event.target.dataset.envKey)changedVariables.set(event.target.dataset.envKey,event.target.value);});
$('environment-form').addEventListener('submit',async event=>{event.preventDefault();if(!environmentDocument)return;
 const changes=Object.fromEntries(changedVariables),key=$('env-new-key').value.trim();if(key)changes[key]=$('env-new-value').value;
 try{environmentDocument=await post(`/api/projects/${toolProject}/environment`,{revision:environmentDocument.revision,changes});await loadEnvironment();toolNotice('Saved changed variables. Existing secrets were kept. Restart to apply.');}catch(e){toolNotice(e.message);}
});
// Tokenize arguments without a shell. Quoted groups are supported; no expansion.
function argumentsList(text){const result=[];let word='',quote='',escaped=false,started=false;for(const c of text){if(escaped){word+=c;escaped=false;started=true;}else if(c==='\\'&&quote!=="'"){escaped=true;started=true;}else if(quote){if(c===quote)quote='';else word+=c;}else if(c==='"'||c==="'"){quote=c;started=true;}else if(/\s/.test(c)){if(started){result.push(word);word='';started=false;}}else{word+=c;started=true;}}if(escaped||quote)throw new Error('Close the quote or escape in the arguments.');if(started)result.push(word);return result;}
$('action-kind').addEventListener('change',()=>{$('action-command').value=$('action-kind').value==='artisan'?'about':'install';$('schema-confirm').checked=false;});
$('action-form').addEventListener('submit',async event=>{event.preventDefault();try{const args=[$('action-command').value.trim(),...argumentsList($('action-args').value)];const task=await post(`/api/projects/${toolProject}/tasks`,{kind:$('action-kind').value,args,confirm_schema:$('schema-confirm').checked});$('schema-confirm').checked=false;watchTask(task.id);$('task-panel').scrollIntoView({behavior:'smooth',block:'start'});}catch(e){toolNotice(e.message);}});
async function watchTask(id){
 currentTask=id;sessionStorage.setItem('laraserve-task',id);clearTimeout(taskTimer);$('task-panel').hidden=false;
 try{const task=await request(`/api/tasks/${id}`);if(currentTask!==id)return;
  $('task-title').textContent=task.kind+' · '+task.id;$('task-status').textContent=task.status+(task.exit_code>=0?' · exit '+task.exit_code:'')+(task.error?' · '+task.error:'');
  const output=$('task-output'),atBottom=output.scrollHeight-output.scrollTop-output.clientHeight<35;output.textContent=task.output||'Waiting for output…';if(atBottom)output.scrollTop=output.scrollHeight;
  const running=['queued','running','recovering'].includes(task.status);$('cancel-task').disabled=!running||task.status==='recovering';
  if(running)taskTimer=setTimeout(()=>watchTask(id),700);else{await refresh();}
 }catch(e){$('task-status').textContent=e.message;}
}
$('cancel-task').addEventListener('click',async()=>{if(!currentTask)return;try{await post(`/api/tasks/${currentTask}/cancel`,{});watchTask(currentTask);}catch(e){message(e.message);}});
$('new-project').addEventListener('click',()=>{$('creation-tools').hidden=false;$('creation-tools').scrollIntoView({behavior:'smooth',block:'start'});});
$('close-creation').addEventListener('click',()=>{$('creation-tools').hidden=true;});
$('create-form').addEventListener('submit',async event=>{event.preventDefault();try{const task=await post('/api/create',{path:$('create-path').value.trim(),starter:$('create-starter').value,database:$('create-database').value,php:$('create-php').value.trim(),version:$('create-version').value.trim(),migrate:$('create-migrate').checked});watchTask(task.id);$('task-panel').scrollIntoView({behavior:'smooth',block:'start'});}catch(e){message(e.message);}});
function runtimeVersions(){$('runtime-version').innerHTML=(catalog[$('runtime-kind').value]||[]).map(v=>`<option value="${escape(v)}">${escape(v)}</option>`).join('');}
async function loadRuntimes(){try{const data=await request('/api/runtimes');$('runtime-list').innerHTML=data.installed.length?data.installed.map(entry=>`<div class="runtime-entry"><div><code>${escape(entry.key)}</code><p>${bytes(entry.bytes)} · ${escape(entry.reason||'Unreferenced published cache')}</p></div>${entry.protected?'':`<button class="button" data-remove-cache="${escape(entry.key)}">Remove</button>`}</div>`).join(''):'<p>No published runtimes in the cache yet.</p>';}catch(e){message(e.message);}}
$('runtime-tools').addEventListener('click',()=>{$('runtime-panel').hidden=false;loadRuntimes();$('runtime-panel').scrollIntoView({behavior:'smooth',block:'start'});});
$('close-runtimes').addEventListener('click',()=>{$('runtime-panel').hidden=true;});
$('runtime-kind').addEventListener('change',runtimeVersions);
$('refresh-runtimes').addEventListener('click',loadRuntimes);
$('runtime-install').addEventListener('submit',async event=>{event.preventDefault();try{const task=await post('/api/runtimes/install',{kind:$('runtime-kind').value,version:$('runtime-version').value});watchTask(task.id);}catch(e){message(e.message);}});
$('runtime-list').addEventListener('click',async event=>{const button=event.target.closest('[data-remove-cache]');if(!button)return;if(!confirm('Remove this unreferenced runtime cache? Projects and databases will be kept.'))return;try{await post('/api/runtimes/remove',{key:button.dataset.removeCache,confirm:true});await loadRuntimes();}catch(e){message(e.message);}});
connect();
