'use strict';
const $ = id => document.getElementById(id);
const escape = value => String(value ?? '').replace(/[&<>"']/g, c => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
const bytes = n => n >= 1073741824 ? (n / 1073741824).toFixed(1) + ' GB' : (n / 1048576).toFixed(0) + ' MB';
let csrf='',projects=[],fetching=false,logRequest,searchTimer;
let currentView='projects',detailTab='overview',settingsGroup='connection',lastRoute='#projects';
let settingsDirty=false,environmentDirty=false,environmentLoaded=false,environmentLoading=false,settingsLoading=false,savingSettings=false,savingEnvironment=false,taskDismissed=false;
const localBusy=new Set();
const attention=['failed','crashed','unresponsive','unknown'];
const projectTabs=['overview','configuration','environment','actions','logs'];
const mediaTheme=window.matchMedia('(prefers-color-scheme: dark)');
let appearance='light';
try{const cookie=document.cookie.split('; ').find(value=>value.startsWith('laraserve-appearance='))?.split('=')[1];const saved=cookie||localStorage.getItem('laraserve-appearance');if(['light','dark','system'].includes(saved))appearance=saved;}catch{}
function applyTheme(){document.documentElement.dataset.theme=appearance==='system'?(mediaTheme.matches?'dark':'light'):appearance;$('theme').value=appearance;}
applyTheme();
$('theme').addEventListener('change',()=>{appearance=$('theme').value;try{localStorage.setItem('laraserve-appearance',appearance);document.cookie=`laraserve-appearance=${appearance}; Max-Age=31536000; Path=/; SameSite=Strict`;}catch{}applyTheme();});
mediaTheme.addEventListener('change',()=>{if(appearance==='system')applyTheme();});
function message(text,error=false){$('notice').textContent=text;$('notice').hidden=!text;$('notice').classList.toggle('danger',error);}
function setFormEnabled(id,enabled){for(const el of $(id).elements)el.disabled=!enabled;}
function updateHTML(el,html){if(el.innerHTML!==html)el.innerHTML=html;}
function safeLink(value){try{const u=new URL(value);return ['http:','https:'].includes(u.protocol)&&!u.username&&!u.password?u.href:'';}catch{return '';}}
function badge(state){return `<span class="badge ${escape(state)}"><i class="dot ${escape(state)}"></i>${escape(state)}</span>`;}
function projectActions(p,compact=false){
 const active=['running','starting','stopping','unresponsive'].includes(p.status),pending=p.operation?.pending||localBusy.has(p.id);
 const disabled=pending?' disabled':'';
 const url=p.status==='running'?safeLink(p.url):'';
 return `<button class="button ${active?'':'primary'}" data-action="${active?'stop':'start'}" data-id="${p.id}"${disabled}>${pending?'Working…':active?'Stop':'Start'}</button>`+
 (!compact&&active?`<button class="button" data-action="restart" data-id="${p.id}"${disabled}>↻ Restart</button>`:'')+
 (url&&!compact?`<a class="button primary" href="${escape(url)}" target="_blank" rel="noopener noreferrer">Open app ↗</a>`:'');
}
function projectRow(p){
 const running=p.services?.filter(s=>s.state==='running').length||0;
 return `<tr data-project="${p.id}"><td><div class="directory-identity"><span class="project-avatar">${escape(p.name.slice(0,1).toUpperCase())}</span><div><a class="project-name" href="#project/${p.id}/overview">${escape(p.name)}</a><span class="source-path" title="${escape(p.source)}">${escape(p.source)}</span></div></div></td><td><div class="stack-label">PHP ${escape(p.php||'Auto')}</div><span class="muted">${escape(p.database||'Auto database')}</span></td><td><span class="service-count">${p.services?.length?`${running} / ${p.services.length} running`:'—'}</span></td><td>${badge(p.status)}${p.error||p.operation?.error?'<span class="row-error">Check project details</span>':''}</td><td><div class="row-actions">${projectActions(p,true)}<a class="button icon-button" href="#project/${p.id}/overview" aria-label="Manage ${escape(p.name)}">↗</a></div></td></tr>`;
}
function syncRows(shown){
 const list=$('project-list'),existing=new Map([...list.children].map(n=>[n.dataset.project,n]));
 for(const [index,p] of shown.entries()){
  const template=document.createElement('template');template.innerHTML=projectRow(p);
  const next=template.content.firstElementChild;let row=existing.get(p.id);
  if(!row)row=next;
  else for(let i=0;i<next.children.length;i++)updateHTML(row.children[i],next.children[i].innerHTML);
  existing.delete(p.id);if(list.children[index]!==row)list.insertBefore(row,list.children[index]||null);
 }
 for(const row of existing.values())row.remove();
}
function render(){
 const query=$('project-search').value.toLowerCase(),status=$('project-status').value;
 const shown=projects.filter(p=>`${p.name} ${p.source} ${p.url}`.toLowerCase().includes(query)&&(status==='all'||(status==='attention'?attention.includes(p.status):p.status===status)));
 syncRows(shown);
 $('total').textContent=$('nav-count').textContent=projects.length;
 $('project-count').textContent=`${shown.length} ${shown.length===1?'project':'projects'}`;
 $('running').textContent=projects.filter(p=>p.status==='running').length;
 const measured=projects.filter(p=>p.resources.available);
 $('memory').textContent=measured.length?bytes(measured.reduce((sum,p)=>sum+p.resources.rss_bytes,0)):'—';
 $('empty').hidden=projects.length!==0;$('no-match').hidden=!projects.length||shown.length!==0;
 document.querySelector('.table-scroll').hidden=shown.length===0;
 const signature=projects.map(p=>p.id+p.name).join('|');
 if($('log-project').dataset.signature!==signature){
  for(const id of ['log-project','project-switcher']){
   const select=$(id),previous=select.value;
   select.innerHTML=(id==='log-project'?'<option value="">Select a project</option>':'')+projects.map(p=>`<option value="${p.id}">${escape(p.name)}</option>`).join('');
   select.value=previous;
  }
  $('log-project').dataset.signature=signature;
 }
 if(currentView==='detail')renderDetail();
}
function renderDetail(){
 const p=projects.find(p=>p.id===toolProject);if(!p)return;
 $('tools-title').textContent=p.name;$('detail-icon').textContent=p.name.slice(0,1).toUpperCase();
 $('detail-path').textContent=p.source;$('detail-path').title=p.source;
 updateHTML($('detail-status'),`<i class="dot ${escape(p.status)}"></i>${escape(p.status)}`);$('detail-status').className='badge '+p.status;
 updateHTML($('detail-actions'),projectActions(p));$('project-switcher').value=p.id;
 const error=p.error||p.operation?.error||'';$('project-error').textContent=error;$('project-error').hidden=!error;
 $('overview-health').textContent=p.health;
 const c=configDocument?.config;
 const facts=[['Application URL',p.url||'Available after starting'],['PHP version',p.php||c?.php||'Detected from project'],['Database',p.database||c?.database||'Detected from project'],['Memory',p.resources.available?bytes(p.resources.rss_bytes):'Not available'],['CPU',p.resources.available?(p.resources.cpu_percent==null?'Sampling…':p.resources.cpu_percent.toFixed(1)+'%'):'Not available'],['Project folder',p.source]];
 updateHTML($('overview-facts'),facts.map(([key,value])=>`<div><dt>${key}</dt><dd>${escape(value)}</dd></div>`).join(''));
 const services=[['vite','Vite','Frontend development server'],['queue','Queue worker','Background jobs'],['scheduler','Scheduler','Scheduled commands'],['redis','Redis','Private data store'],['mailpit','Mailpit','Local email inbox']];
 updateHTML($('overview-services'),services.map(([key,label,description])=>{
  const service=p.services?.find(s=>s.name===key),enabled=c?.services?.[key],state=service?.state||(enabled?'stopped':c?'off':'unknown');
  return `<div class="service-item"><span class="service-icon">${label.slice(0,1)}</span><div><strong>${label}</strong><small>${description}</small></div>${badge(state)}</div>`;
 }).join(''));
 document.querySelectorAll('[data-project-tab]').forEach(a=>{a.href=`#project/${p.id}/${a.dataset.projectTab}`;if(a.dataset.projectTab===detailTab)a.setAttribute('aria-current','page');else a.removeAttribute('aria-current');});
 document.querySelectorAll('[data-shortcut]').forEach(a=>a.href=`#project/${p.id}/${a.dataset.shortcut==='services'?'configuration/services':a.dataset.shortcut}`);
}
function selectSettingsGroup(name){
 settingsGroup=['connection','runtimes','php','services'].includes(name)?name:'connection';
 document.querySelectorAll('[data-setting-section]').forEach(el=>el.hidden=el.dataset.settingSection!==settingsGroup);
 document.querySelectorAll('[data-settings-group]').forEach(el=>el.setAttribute('aria-pressed',String(el.dataset.settingsGroup===settingsGroup)));
}
function hasDraft(){return settingsDirty||environmentDirty;}
function route(){
 if(!csrf)return;
 const hash=location.hash||'#projects',parts=hash.slice(1).split('/');
 const isProject=parts[0]==='project'&&/^[a-f0-9]{12}$/.test(parts[1]||'');
 const changesProject=!isProject||parts[1]!==toolProject;
 if(savingSettings||savingEnvironment||(changesProject&&hasDraft()&&!confirm('Discard your unsaved project changes?'))){history.replaceState(null,'',lastRoute);$('project-switcher').value=toolProject;return;}
 lastRoute=hash;message('');
 if(changesProject){if(logRequest){logRequest.abort();logRequest=undefined;}settingsDirty=false;environmentDirty=false;toolProject='';configDocument=undefined;environmentDocument=undefined;environmentLoaded=false;environmentLoading=false;toolNotice('');}
 currentView=isProject?'detail':['projects','runtimes','new','activity'].includes(parts[0])?parts[0]:'projects';
 if(isProject&&!projects.some(p=>p.id===parts[1])){currentView='projects';message('This project is no longer registered.');}
 document.querySelectorAll('[data-view]').forEach(el=>el.hidden=el.dataset.view!==currentView);
 document.querySelectorAll('[data-nav]').forEach(el=>{if(el.dataset.nav===(currentView==='detail'||currentView==='new'?'projects':currentView))el.setAttribute('aria-current','page');else el.removeAttribute('aria-current');});
 if(currentView==='detail'){
  const nextTab=projectTabs.includes(parts[2])?parts[2]:'overview';if(nextTab!==detailTab)toolNotice('');detailTab=nextTab;
  document.querySelectorAll('[data-project-panel]').forEach(el=>el.hidden=el.dataset.projectPanel!==detailTab);
  if(toolProject!==parts[1]||(!configDocument&&!settingsLoading))openTools(parts[1]);
  selectSettingsGroup(parts[3]||settingsGroup);renderDetail();
  if(detailTab==='environment'&&!environmentLoaded&&!environmentLoading)loadEnvironment();
  if(detailTab==='logs'){$('log-project').value=parts[1];loadLogs();}
  document.title=`${projects.find(p=>p.id===parts[1]).name} · LaraServe`;
 }else{
  document.title=`${{projects:'Projects',runtimes:'Runtimes',new:'Create app',activity:'Activity'}[currentView]} · LaraServe`;
  if(currentView==='runtimes')loadRuntimes();
 }
 if(currentView==='activity'){$('activity-empty').hidden=!!currentTask;if(currentTask){taskDismissed=false;$('task-panel').hidden=false;}}
 // Move focus to the new page title for keyboard users without disturbing live refreshes.
 const heading=document.querySelector(`[data-view="${currentView}"] h1`);if(heading){heading.tabIndex=-1;heading.focus({preventScroll:true});}
 window.scrollTo({top:0,behavior:'instant'});
}
async function request(path,options={}){
 const response=await fetch(path,{cache:'no-store',credentials:'same-origin',...options});
 if(!response.ok){if(response.status===401){$('locked').hidden=false;$('workspace').hidden=true;}throw new Error((await response.text()).trim()||'Request failed');}
 return response.status===202||response.status===204?null:response.json();
}
async function refresh(){
 if(fetching||!csrf)return;fetching=true;
 try{projects=await request('/api/projects');render();$('updated').textContent='Updated '+new Date().toLocaleTimeString([],{hour:'2-digit',minute:'2-digit',second:'2-digit'});}
 catch(e){message(e.message,true);}finally{fetching=false;}
}
async function loadLogs(){
 if(logRequest)logRequest.abort();
 const id=$('log-project').value;if(!id)return;
 if($('log-output').dataset.project!==id){$('log-output').textContent='Loading this project’s output…';$('log-output').dataset.project=id;}
 const controller=new AbortController();logRequest=controller;$('log-state').textContent='Reading recent output…';
 try{
  const data=await request(`/api/projects/${id}/logs?`+new URLSearchParams({source:$('log-source').value,q:$('log-search').value}),{signal:controller.signal});
  if(controller!==logRequest)return;
  $('log-output').textContent=data.text||($('log-search').value?'No matching lines in recent output.':'No output in this log yet.');
  $('log-note').textContent=data.note;$('log-state').textContent=data.truncated?'Recent tail · output truncated':'Updated '+new Date().toLocaleTimeString();
 }catch(e){if(e.name!=='AbortError'&&controller===logRequest){$('log-output').textContent=e.message;$('log-state').textContent='Could not read log';}}
}
document.querySelector('.skip-link').addEventListener('click',event=>{event.preventDefault();$('main-content').focus();$('main-content').scrollIntoView();});
document.addEventListener('click',async event=>{
 const button=event.target.closest('[data-action]');if(!button)return;
 const id=button.dataset.id,action=button.dataset.action;
 if(localBusy.has(id))return;
 localBusy.add(id);render();message('');
 try{await post(`/api/projects/${id}/${action}`,{});}catch(e){message(e.message,true);}finally{localBusy.delete(id);await refresh();}
});
$('refresh').addEventListener('click',()=>{message('');refresh();});
$('project-search').addEventListener('input',render);$('project-status').addEventListener('change',render);
$('clear-filters').addEventListener('click',()=>{$('project-search').value='';$('project-status').value='all';render();$('project-search').focus();});
$('project-switcher').addEventListener('change',()=>{location.hash=`project/${$('project-switcher').value}/${detailTab}`;});
$('log-source').addEventListener('change',loadLogs);$('refresh-logs').addEventListener('click',loadLogs);
$('log-search').addEventListener('input',()=>{clearTimeout(searchTimer);searchTimer=setTimeout(loadLogs,250);});
document.querySelectorAll('[data-settings-group]').forEach(button=>button.addEventListener('click',()=>{selectSettingsGroup(button.dataset.settingsGroup);history.replaceState(null,'',`#project/${toolProject}/configuration/${settingsGroup}`);lastRoute=location.hash;}));
$('settings-form').addEventListener('invalid',event=>{const section=event.target.closest('[data-setting-section]');if(section)selectSettingsGroup(section.dataset.settingSection);},true);
$('settings-form').addEventListener('input',()=>{settingsDirty=true;$('settings-save-state').textContent='Unsaved changes';toolNotice('');});
$('settings-form').addEventListener('change',()=>{settingsDirty=true;$('settings-save-state').textContent='Unsaved changes';toolNotice('');});
$('environment-form').addEventListener('input',()=>{environmentDirty=true;toolNotice('');});
document.querySelectorAll('[data-command]').forEach(button=>button.addEventListener('click',()=>{$('action-kind').value='artisan';$('action-command').value=button.dataset.command;$('action-args').value='';$('schema-confirm').checked=false;$('action-command').focus();}));
window.addEventListener('beforeunload',event=>{if(hasDraft()||savingSettings||savingEnvironment){event.preventDefault();event.returnValue='';}});
window.addEventListener('hashchange',()=>{if(/^[a-f0-9]{64}$/.test(location.hash.slice(1)))connect();else route();});
async function connect(){
 const token=location.hash.slice(1);if(/^[a-f0-9]{64}$/.test(token))history.replaceState(null,'','#projects');
 try{
  if(/^[a-f0-9]{64}$/.test(token))await request('/session',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({token})});
  const session=await request('/api/session');csrf=session.csrf;message('');$('locked').hidden=true;$('workspace').hidden=false;
  await refresh();route();try{await loadCatalog();}catch(e){message('Runtime catalog unavailable. '+e.message,true);}
 }catch(e){$('locked').hidden=false;$('workspace').hidden=true;}
}
setInterval(()=>{if(!document.hidden)refresh();},3000);
let toolProject = '', configDocument, environmentDocument, catalog = {php:[],mysql:[]}, currentTask = '', taskTimer;
const changedVariables = new Map();
const phpFields=['memory_limit','max_execution_time','upload_max_filesize','post_max_size','date.timezone','display_errors'];
function post(path, body) {return request(path,{method:'POST',headers:{'Content-Type':'application/json','X-CSRF-Token':csrf},body:JSON.stringify(body)});}
async function loadCatalog(){catalog=await request('/api/catalog');for(const kind of ['php','mysql'])$(kind+'-versions').innerHTML=catalog[kind].map(version=>`<option value="${escape(version)}"></option>`).join('');runtimeVersions();let previous;try{previous=sessionStorage.getItem('laraserve-task');}catch{}if(previous&&/^[a-f0-9]{12}$/.test(previous)&&!currentTask)watchTask(previous);}
function toolNotice(text,error=false){$('tools-notice').textContent=text;$('tools-notice').hidden=!text;$('tools-notice').classList.toggle('danger',error);}
async function openTools(id){
 const p=projects.find(p=>p.id===id);
 if(!p)return;
 const changed=toolProject!==id;toolProject=id;settingsLoading=true;configDocument=undefined;$('settings-form').reset();$('settings-save-state').textContent='Changes apply after a restart.';
 settingsDirty=false;if(changed){environmentDocument=undefined;environmentLoaded=false;environmentDirty=false;setFormEnabled('environment-form',false);}
 setFormEnabled('settings-form',false);
 toolNotice('Loading configuration…');
 try{
  const loaded=await request(`/api/projects/${id}/settings`);
  if(toolProject!==id)return;
  configDocument=loaded;const c=loaded.config,f=$('settings-form').elements;
  for(const key of ['domain','php','mysql','node','php_ini'])f[key].value=c[key]||'';
  for(const key of ['vite','queue','scheduler','redis','mailpit'])f['service_'+key].checked=!!c.services?.[key];
  f.port.value=c.port??'';f.database.value=c.database||'auto';f.https.value=c.https==null?'':String(c.https);f.phpmyadmin.value=c.phpmyadmin==null?'':String(c.phpmyadmin);
  for(const key of phpFields)f[key].value=c.php_settings?.[key]||'';
  setFormEnabled('settings-form',true);toolNotice('');renderDetail();
 }catch(e){if(toolProject===id)toolNotice(e.message,true);}finally{if(toolProject===id)settingsLoading=false;}
}
async function loadEnvironment(){
 const id=toolProject;environmentLoading=true;environmentDocument=undefined;$('environment-fields').replaceChildren();setFormEnabled('environment-form',false);toolNotice('Loading masked environment…');changedVariables.clear();$('env-new-key').value='';$('env-new-value').value='';
 try{
  const loaded=await request(`/api/projects/${id}/environment`);if(toolProject!==id)return;environmentDocument=loaded;
  $('environment-fields').innerHTML=environmentDocument.variables.map(v=>`<div class="env-row"><label>${escape(v.key)} ${v.secret?'· masked':''}<input data-env-key="${escape(v.key)}" type="${v.secret?'password':'text'}" value="${escape(v.value)}" placeholder="${v.secret&&v.present?'Unchanged secret':v.present?'Empty value':'Not in .env'}" autocomplete="off"></label><small>${v.in_example?'In .env.example':'Only in .env'}${v.secret?' · example value hidden':v.in_example?'<br>Example: '+escape(v.example):''}${!v.present?' · missing from .env':''}</small></div>`).join('');
  $('env-comparison').textContent=environmentDocument.variables.filter(v=>v.in_example&&!v.present).length+' example variables missing';environmentLoaded=true;environmentDirty=false;setFormEnabled('environment-form',true);toolNotice('');
 }catch(e){if(toolProject===id)toolNotice(e.message,true);}finally{if(toolProject===id)environmentLoading=false;}
}
$('settings-form').addEventListener('submit',async event=>{
 event.preventDefault();if(!configDocument||savingSettings)return;const f=event.target.elements,c={...configDocument.config};
 for(const key of ['domain','php','mysql','node','php_ini'])c[key]=f[key].value.trim();
 c.services={...c.services};for(const key of ['vite','queue','scheduler','redis','mailpit'])c.services[key]=f['service_'+key].checked;
 c.database=f.database.value;for(const key of ['https','phpmyadmin']){if(f[key].value==='')delete c[key];else c[key]=f[key].value==='true';}
 if(f.port.value==='')delete c.port;else c.port=Number(f.port.value);
 c.php_settings={...c.php_settings};for(const key of phpFields){if(f[key].value==='')delete c.php_settings[key];else c.php_settings[key]=f[key].value.trim();}
 savingSettings=true;setFormEnabled('settings-form',false);
 try{configDocument=await post(`/api/projects/${toolProject}/settings`,{revision:configDocument.revision,config:c});settingsDirty=false;$('settings-save-state').textContent='All changes saved';toolNotice('Configuration saved. Restart the project to apply your changes.');renderDetail();}catch(e){toolNotice(e.message,true);}finally{savingSettings=false;setFormEnabled('settings-form',true);}
});
$('environment-fields').addEventListener('input',event=>{if(event.target.dataset.envKey)changedVariables.set(event.target.dataset.envKey,event.target.value);});
$('environment-form').addEventListener('submit',async event=>{event.preventDefault();if(!environmentDocument||savingEnvironment)return;
 const changes=Object.fromEntries(changedVariables),key=$('env-new-key').value.trim();if(key)changes[key]=$('env-new-value').value;
 savingEnvironment=true;setFormEnabled('environment-form',false);
 try{await post(`/api/projects/${toolProject}/environment`,{revision:environmentDocument.revision,changes});environmentDirty=false;await loadEnvironment();toolNotice('Environment saved. Restart the project to apply your changes.');}catch(e){toolNotice(e.message,true);}finally{savingEnvironment=false;if(environmentDocument)setFormEnabled('environment-form',true);}
});
// Tokenize arguments without a shell. Quoted groups are supported; no expansion.
function argumentsList(text){const result=[];let word='',quote='',escaped=false,started=false;for(const c of text){if(escaped){word+=c;escaped=false;started=true;}else if(c==='\\'&&quote!=="'"){escaped=true;started=true;}else if(quote){if(c===quote)quote='';else word+=c;}else if(c==='"'||c==="'"){quote=c;started=true;}else if(/\s/.test(c)){if(started){result.push(word);word='';started=false;}}else{word+=c;started=true;}}if(escaped||quote)throw new Error('Close the quote or escape in the arguments.');if(started)result.push(word);return result;}
$('action-kind').addEventListener('change',()=>{$('action-command').value=$('action-kind').value==='artisan'?'about':'install';$('schema-confirm').checked=false;});
$('action-form').addEventListener('submit',async event=>{event.preventDefault();if(event.target.dataset.pending)return;event.target.dataset.pending='true';setFormEnabled('action-form',false);try{const args=[$('action-command').value.trim(),...argumentsList($('action-args').value)];const task=await post(`/api/projects/${toolProject}/tasks`,{kind:$('action-kind').value,args,confirm_schema:$('schema-confirm').checked});$('schema-confirm').checked=false;watchTask(task.id);$('task-panel').scrollIntoView({behavior:'smooth',block:'start'});}catch(e){toolNotice(e.message,true);}finally{delete event.target.dataset.pending;setFormEnabled('action-form',true);}});
async function watchTask(id,reveal=true){
 currentTask=id;try{sessionStorage.setItem('laraserve-task',id);}catch{}clearTimeout(taskTimer);if(reveal)taskDismissed=false;$('task-panel').hidden=taskDismissed;$('activity-empty').hidden=true;
 try{const task=await request(`/api/tasks/${id}`);if(currentTask!==id)return;
  $('task-title').textContent=task.kind+' · '+task.id;$('task-status').textContent=task.status+(task.exit_code>=0?' · exit '+task.exit_code:'')+(task.error?' · '+task.error:'');
  const output=$('task-output'),atBottom=output.scrollHeight-output.scrollTop-output.clientHeight<35;output.textContent=task.output||'Waiting for output…';if(atBottom)output.scrollTop=output.scrollHeight;
  const running=['queued','running','recovering'].includes(task.status);$('activity-dot').hidden=!running;$('cancel-task').disabled=!running||task.status==='recovering';
  if(running)taskTimer=setTimeout(()=>watchTask(id,false),700);else{await refresh();}
 }catch(e){$('task-status').textContent=e.message;}
}
$('cancel-task').addEventListener('click',async()=>{if(!currentTask)return;try{await post(`/api/tasks/${currentTask}/cancel`,{});watchTask(currentTask);}catch(e){message(e.message,true);}});
$('create-form').addEventListener('submit',async event=>{event.preventDefault();if(event.target.dataset.pending)return;event.target.dataset.pending='true';setFormEnabled('create-form',false);try{const task=await post('/api/create',{path:$('create-path').value.trim(),starter:$('create-starter').value,database:$('create-database').value,php:$('create-php').value.trim(),version:$('create-version').value.trim(),migrate:$('create-migrate').checked});watchTask(task.id);$('task-panel').scrollIntoView({behavior:'smooth',block:'start'});}catch(e){message(e.message,true);}finally{delete event.target.dataset.pending;setFormEnabled('create-form',true);}});
function runtimeVersions(){$('runtime-version').innerHTML=[...(catalog[$('runtime-kind').value]||[])].reverse().map(v=>`<option value="${escape(v)}">${escape(v)}</option>`).join('');}
async function loadRuntimes(){try{const data=await request('/api/runtimes');$('runtime-list').innerHTML=data.installed.length?data.installed.map(entry=>`<div class="runtime-entry"><div><code>${escape(entry.key)}</code><p>${bytes(entry.bytes)} · ${escape(entry.reason||'Unreferenced published cache')}</p></div>${entry.protected?'':`<button class="button" data-remove-cache="${escape(entry.key)}">Remove</button>`}</div>`).join(''):'<p>No published runtimes in the cache yet.</p>';}catch(e){message(e.message,true);}}
$('runtime-kind').addEventListener('change',runtimeVersions);
$('refresh-runtimes').addEventListener('click',loadRuntimes);
$('runtime-install').addEventListener('submit',async event=>{event.preventDefault();if(event.target.dataset.pending)return;event.target.dataset.pending='true';setFormEnabled('runtime-install',false);try{const task=await post('/api/runtimes/install',{kind:$('runtime-kind').value,version:$('runtime-version').value});watchTask(task.id);$('task-panel').scrollIntoView({behavior:'smooth',block:'start'});}catch(e){message(e.message,true);}finally{delete event.target.dataset.pending;setFormEnabled('runtime-install',true);}});
$('runtime-list').addEventListener('click',async event=>{const button=event.target.closest('[data-remove-cache]');if(!button)return;if(!confirm('Remove this unreferenced runtime cache? Projects and databases will be kept.'))return;try{await post('/api/runtimes/remove',{key:button.dataset.removeCache,confirm:true});await loadRuntimes();}catch(e){message(e.message,true);}});
$('dismiss-task').addEventListener('click',()=>{
 taskDismissed=true;$('task-panel').hidden=true;
 if(currentView==='activity'){
  $('activity-empty').hidden=false;
  $('activity-empty').querySelector('h2').textContent='Task output hidden';
  $('activity-empty').querySelector('p').textContent='Your latest task is still available. Reopen it to review its output.';
  $('show-task').hidden=false;
 }
});
$('show-task').addEventListener('click',()=>{taskDismissed=false;$('activity-empty').hidden=true;$('task-panel').hidden=false;});
connect();
