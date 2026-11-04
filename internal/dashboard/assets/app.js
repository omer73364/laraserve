'use strict';
const $ = id => document.getElementById(id);
const escape = value => String(value ?? '').replace(/[&<>"']/g, c => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
const bytes = n => n >= 1073741824 ? (n / 1073741824).toFixed(1) + ' GB' : (n / 1048576).toFixed(0) + ' MB';
let csrf='',projects=[],fetching=false,logRequest,searchTimer;
let currentView='projects',detailTab='overview',settingsGroup='connection',lastRoute='#projects';
let settingsDirty=false,environmentDirty=false,environmentLoaded=false,environmentLoading=false,settingsLoading=false,savingSettings=false,savingEnvironment=false,taskDismissed=false;
const localBusy=new Set();
const observedOperations=new Map();
const telemetry={signature:'',cpu:[],memory:[]};
const attention=['failed','crashed','unresponsive','unknown'];
const projectTabs=['overview','configuration','environment','actions','logs'];
const mediaTheme=window.matchMedia('(prefers-color-scheme: dark)');
let appearance='light';
try{const cookie=document.cookie.split('; ').find(value=>value.startsWith('laraserve-appearance='))?.split('=')[1];const saved=cookie||localStorage.getItem('laraserve-appearance');if(['light','dark','system'].includes(saved))appearance=saved;}catch{}
function applyTheme(){document.documentElement.dataset.theme=appearance==='system'?(mediaTheme.matches?'dark':'light'):appearance;$('theme').value=appearance;LaraControls.sync();}
applyTheme();
$('theme').addEventListener('change',()=>{appearance=$('theme').value;try{localStorage.setItem('laraserve-appearance',appearance);document.cookie=`laraserve-appearance=${appearance}; Max-Age=31536000; Path=/; SameSite=Strict`;}catch{}applyTheme();});
mediaTheme.addEventListener('change',()=>{if(appearance==='system')applyTheme();});
function toast(text,error=false){LaraNotifications.show(text,error);}
function message(text,error=false){toast(text,error);}
function setFormEnabled(id,enabled){for(const el of $(id).elements)el.disabled=!enabled;LaraControls.sync();}
function updateHTML(el,html){if(el.innerHTML!==html)el.innerHTML=html;}
function safeLink(value){try{const u=new URL(value);return ['http:','https:'].includes(u.protocol)&&!u.username&&!u.password?u.href:'';}catch{return '';}}
function badge(state){return `<span class="badge ${escape(state)}"><i class="dot ${escape(state)}"></i>${escape(state)}</span>`;}
function projectActions(p,compact=false){
 const active=['running','starting','stopping','unresponsive'].includes(p.status),pending=p.operation?.pending||localBusy.has(p.id);
 const disabled=pending?' disabled':'';
 const url=p.status==='running'?safeLink(p.url):'';
 const restartDisabled=pending||!active||['starting','stopping'].includes(p.status)?' disabled':'';
 return `<button class="button ${active?'':'primary'}" data-action="${active?'stop':'start'}" data-id="${p.id}"${disabled}>${pending?'Working…':active?'Stop':'Start'}</button>`+
 `<button class="button ${compact?'restart-control':''}" data-action="restart" data-id="${p.id}" aria-label="Restart ${escape(p.name)}" title="Restart ${escape(p.name)}"${restartDisabled}>${compact?'↻':'↻ Restart'}</button>`+
 (p.operation?.pending?`<button class="button quiet" data-progress-id="${p.id}" data-progress-action="${escape(p.operation.action)}">Progress</button>`:'')+
 (url?`<a class="button open-app ${compact?'':'primary'}" href="${escape(url)}" target="_blank" rel="noopener noreferrer">Open app <span aria-hidden="true">↗</span></a>`:`<button class="button open-app" disabled title="Start this app to open it">Open app <span aria-hidden="true">↗</span></button>`);
}
function projectRow(p){
 const running=p.services?.filter(s=>s.state==='running'||s.state==='ready').length||0;
 const framework=p.laravel?escape(p.laravel.replace(/^v(?=\d)/,'')):'—';
 return `<tr data-project="${p.id}"><td><div class="directory-identity"><span class="project-avatar">${escape(p.name.slice(0,1).toUpperCase())}</span><div><a class="project-name" href="#project/${p.id}/overview">${escape(p.name)}</a>${p.startup?'<span class="startup-label">At startup</span>':''}<span class="source-path" title="${escape(p.source)}">${escape(p.source)}</span></div></div></td><td><span class="mobile-field">Laravel </span><span class="framework-version" title="${escape(p.laravel_source||'Version unavailable')}">${framework}</span><span class="version-source">${p.laravel?(p.laravel_source==='composer.lock'?'Locked':'Constraint'):'Not detected'}</span></td><td><div class="stack-label">PHP ${escape(p.php||'Auto')}</div><span class="muted">${escape(p.database||'Auto database')}</span></td><td><span class="service-count">${p.services?.length?`${running} / ${p.services.length} running`:'None tracked'}</span></td><td>${badge(p.status)}${p.error||p.operation?.error?'<span class="row-error">Check project details</span>':''}</td><td><div class="row-actions">${projectActions(p,true)}<button class="button remove-control" data-remove="${p.id}" aria-label="Remove ${escape(p.name)}" title="Remove ${escape(p.name)}"${p.operation?.pending||localBusy.has(p.id)?' disabled':''}>×</button></div></td></tr>`;
}
function recordTelemetry(){
 const measured=projects.filter(p=>p.resources.available),signature=measured.map(p=>p.id).sort().join('|');
 if(signature!==telemetry.signature){telemetry.cpu=[];telemetry.memory=[];telemetry.signature=signature;}
 if(!measured.length){telemetry.cpu=[];telemetry.memory=[];return;}
 telemetry.memory.push(measured.reduce((sum,p)=>sum+p.resources.rss_bytes,0));
 const sampled=measured.filter(p=>p.resources.cpu_percent!=null);
 if(sampled.length===measured.length)telemetry.cpu.push(sampled.reduce((sum,p)=>sum+p.resources.cpu_percent,0));else telemetry.cpu=[];
 telemetry.memory=telemetry.memory.slice(-30);telemetry.cpu=telemetry.cpu.slice(-30);
}
function sparkline(values){
 if(values.length<2)return '<span class="trend-placeholder"></span>';
 const max=Math.max(...values,1),points=values.map((value,i)=>`${(i/(values.length-1)*110+2).toFixed(1)},${(26-value/max*22).toFixed(1)}`).join(' ');
 return `<svg viewBox="0 0 114 30" focusable="false"><polyline points="${points}"></polyline></svg>`;
}
function renderMonitoring(){
 const running=projects.filter(p=>p.status==='running'),measured=projects.filter(p=>p.resources.available),sampled=measured.filter(p=>p.resources.cpu_percent!=null);
 const services=projects.flatMap(p=>p.services||[]),active=services.filter(s=>['running','ready'].includes(s.state));
 const issues=projects.filter(p=>attention.includes(p.status));
 $('total').textContent=projects.length;$('running').textContent=running.length;
 $('running-note').textContent=issues.length?`${issues.length} ${issues.length===1?'app needs':'apps need'} attention`:`${projects.filter(p=>p.status==='stopped').length} stopped · ready when you are`;
 updateHTML($('app-dots'),projects.slice(0,14).map(p=>`<i class="dot ${escape(p.status)}"></i>`).join(''));
 $('cpu').textContent=sampled.length?sampled.reduce((sum,p)=>sum+p.resources.cpu_percent,0).toFixed(1)+'%':measured.length?'Sampling…':'—';
 $('cpu-note').textContent=sampled.length?(sampled.length===running.length?`${sampled.length} ${sampled.length===1?'app':'apps'} sampled`:`${sampled.length} of ${running.length} apps sampled`):(measured.length?'Collecting first CPU sample':running.length?'Counters unavailable':'Start an app to monitor');
 $('memory').textContent=measured.length?bytes(measured.reduce((sum,p)=>sum+p.resources.rss_bytes,0)):'—';
 $('memory-note').textContent=measured.length?`${measured.length} ${measured.length===1?'app':'apps'} · process tree RSS`:(running.length?'Counters unavailable':'Start an app to monitor');
 $('active-services').textContent=active.length;$('service-total').textContent=services.length?`of ${services.length} tracked`:'';
 $('service-health').textContent=services.some(s=>['failed','crashed'].includes(s.state))?'Check services':active.length?'Running':'Idle';
 updateHTML($('cpu-trend'),sparkline(telemetry.cpu));updateHTML($('memory-trend'),sparkline(telemetry.memory));
 $('monitor-attention').hidden=!issues.length;$('monitor-attention').textContent=`${issues.length} ${issues.length===1?'app needs':'apps need'} attention. Review status →`;
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
 $('nav-count').textContent=projects.length;
 $('project-count').textContent=`${shown.length} ${shown.length===1?'project':'projects'}`;
 renderMonitoring();
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
 if(currentView==='detail')renderDetail();if(currentView==='detail'&&detailTab==='actions')loadTaskHistory(toolProject);if(currentView==='activity')loadTaskHistory();if($('uninstall-dialog').open)validateUninstall();
 LaraControls.sync();
}
function renderDetail(){
 const p=projects.find(p=>p.id===toolProject);if(!p)return;
 $('tools-title').textContent=p.name;$('detail-icon').textContent=p.name.slice(0,1).toUpperCase();
 if(!startupBusy.has(p.id)){$('project-startup').checked=!!p.startup;$('project-startup').disabled=!!p.operation?.pending;}$('project-startup-note').textContent='For startup at sign-in, enable Launch LaraServe at login in Settings.';
 $('detail-path').textContent=p.source;$('detail-path').title=p.source;
 updateHTML($('detail-status'),`<i class="dot ${escape(p.status)}"></i>${escape(p.status)}`);$('detail-status').className='badge '+p.status;
 updateHTML($('detail-actions'),projectActions(p)+(p.admin_url?`<a class="button" href="${escape(p.admin_url)}" target="_blank" rel="noopener noreferrer">Open phpMyAdmin ↗</a>`:'<button class="button" disabled title="Start this project with private MySQL and phpMyAdmin enabled">Open phpMyAdmin ↗</button>'));$('project-switcher').value=p.id;
 const error=p.error||p.operation?.error||'';$('project-error').textContent=error;$('project-error').hidden=!error;
 $('overview-health').textContent=p.health;
 const c=configDocument?.config;
 const stack=[['PHP',p.php||c?.php||'Automatic','Project runtime'],['Node.js · last selected',p.node||'Not prepared',p.node_reason||'Selected when preparing frontend'],['Database',p.database||c?.database||'Automatic','Project connection'],['Laravel',p.laravel||'Not detected',p.laravel_source==='composer.lock'?'Locked version':'Project requirement']];
 updateHTML($('project-stack'),stack.map(([label,value,note])=>`<article class="stack-card"><span>${escape(label)}</span><strong>${escape(value)}</strong><small title="${escape(note)}">${escape(note)}</small></article>`).join(''));
 const facts=[['Application URL',p.url||'Available after starting'],['Memory',p.resources.available?bytes(p.resources.rss_bytes):'Not available'],['CPU',p.resources.available?(p.resources.cpu_percent==null?'Sampling…':p.resources.cpu_percent.toFixed(1)+'%'):'Not available'],['Project folder',p.source]];
 updateHTML($('overview-facts'),facts.map(([key,value])=>`<div><dt>${key}</dt><dd>${escape(value)}${key==='Application URL'&&safeLink(p.url)?`<button class="button quiet copy-url" data-copy-url="${escape(safeLink(p.url))}" aria-label="Copy application URL">Copy</button>`:''}</dd></div>`).join(''));
 const groups=[['Frontend',[['vite','Vite','Live frontend updates']]],['Background tasks',[['queue','Queue worker','Processes background jobs'],['scheduler','Scheduler','Runs scheduled commands']]],['Data & mail',[['redis','Redis','Local cache and queue storage'],['mailpit','Mailpit','Captured development email']]]];
 updateHTML($('overview-services'),groups.map(([title,services])=>`<section class="service-group"><h3>${title}</h3>${services.map(([key,label,description])=>{
  const service=p.services?.find(s=>s.name===key),enabled=c?.services?.[key],automatic=c&&c.services?.auto_detect!==false&&['vite','redis','mailpit'].includes(key),state=service?.state||(enabled?'stopped':automatic?'automatic':c?'off':'unknown'),url=['running','ready'].includes(state)?safeLink(service?.url):'';
  return `<div class="service-item"><span class="service-icon" aria-hidden="true">${label.slice(0,1)}</span><div class="service-description"><strong>${label}</strong><small>${description}</small></div><div class="service-controls">${badge(state)}${url?`<a class="service-link" href="${escape(url)}" target="_blank" rel="noopener noreferrer" aria-label="Open ${label}">Open ↗</a>`:`<a class="service-link" href="#project/${p.id}/configuration/services" aria-label="Configure ${label}">Configure</a>`}</div></div>`;
 }).join('')}</section>`).join(''));
 document.querySelectorAll('[data-project-tab]').forEach(a=>{a.href=`#project/${p.id}/${a.dataset.projectTab}`;if(a.dataset.projectTab===detailTab)a.setAttribute('aria-current','page');else a.removeAttribute('aria-current');});
 document.querySelectorAll('[data-shortcut]').forEach(a=>a.href=`#project/${p.id}/${a.dataset.shortcut==='services'?'configuration/services':a.dataset.shortcut}`);
}
function selectSettingsGroup(name){
 settingsGroup=['connection','runtimes','php','services'].includes(name)?name:'connection';
 document.querySelectorAll('[data-setting-section]').forEach(el=>el.hidden=el.dataset.settingSection!==settingsGroup);
 document.querySelectorAll('[data-settings-group]').forEach(el=>el.setAttribute('aria-pressed',String(el.dataset.settingsGroup===settingsGroup)));
}
function hasDraft(){return settingsDirty||environmentDirty||applicationDirty;}
function route(){
 if(!csrf)return;
 const hash=location.hash||'#projects',parts=hash.slice(1).split('/');
 const isProject=parts[0]==='project'&&/^[a-f0-9]{12}$/.test(parts[1]||'');
 const changesProject=!isProject||parts[1]!==toolProject;
 if(uninstalling||applicationSaving||savingSettings||savingEnvironment||(changesProject&&hasDraft()&&!confirm('Discard your unsaved changes?'))){history.replaceState(null,'',lastRoute);$('project-switcher').value=toolProject;return;}
 lastRoute=hash;applicationDirty=false;message('');LaraControls.close();
 if(changesProject){if(logRequest){logRequest.abort();logRequest=undefined;}settingsDirty=false;environmentDirty=false;toolProject='';configDocument=undefined;environmentDocument=undefined;environmentLoaded=false;environmentLoading=false;toolNotice('');}
 currentView=isProject?'detail':['projects','runtimes','new','add','activity','application'].includes(parts[0])?parts[0]:'projects';
 if(isProject&&!projects.some(p=>p.id===parts[1])){currentView='projects';message('This project is no longer registered.');}
 document.querySelectorAll('[data-view]').forEach(el=>el.hidden=el.dataset.view!==currentView);
 document.querySelectorAll('[data-nav]').forEach(el=>{if(el.dataset.nav===(currentView==='detail'||currentView==='new'||currentView==='add'?'projects':currentView))el.setAttribute('aria-current','page');else el.removeAttribute('aria-current');});
 if(currentView==='detail'){
  const nextTab=projectTabs.includes(parts[2])?parts[2]:'overview';if(nextTab!==detailTab)toolNotice('');detailTab=nextTab;
  document.querySelectorAll('[data-project-panel]').forEach(el=>el.hidden=el.dataset.projectPanel!==detailTab);
  if(toolProject!==parts[1]||(!configDocument&&!settingsLoading))openTools(parts[1]);
  selectSettingsGroup(parts[3]||settingsGroup);renderDetail();
  if(detailTab==='environment'&&!environmentLoaded&&!environmentLoading)loadEnvironment();
  if(detailTab==='actions')loadTaskHistory(toolProject);
 if(detailTab==='logs'){$('logs').querySelector('h2').textContent='Logs · '+projects.find(p=>p.id===parts[1]).name;$('log-project').value=parts[1];loadLogs();}
  document.title=`${projects.find(p=>p.id===parts[1]).name} · LaraServe`;
 }else{
  document.title=`${{projects:'Projects',runtimes:'Runtimes',new:'Create app',add:'Add app',activity:'Activity',application:'Settings'}[currentView]} · LaraServe`;
  if(currentView==='runtimes')loadRuntimes();if(currentView==='application')loadApplication();
 }
 if(currentView==='activity'){loadTaskHistory();$('activity-empty').hidden=!!currentTask;if(currentTask){taskDismissed=false;$('task-panel').hidden=false;}}
 $('task-panel').hidden=currentView!=='activity'||taskDismissed;
 // Move focus to the new page title for keyboard users without disturbing live refreshes.
 const heading=document.querySelector(`[data-view="${currentView}"] h1`);if(heading){heading.tabIndex=-1;heading.focus({preventScroll:true});}
 LaraControls.sync();window.scrollTo({top:0,behavior:'instant'});
}
async function request(path,options={}){
 const response=await fetch(path,{cache:'no-store',credentials:'same-origin',...options});
 if(!response.ok){if(response.status===401){$('locked').hidden=false;$('workspace').hidden=true;}throw new Error((await response.text()).trim()||'Request failed');}
 return response.status===202||response.status===204?null:response.json();
}
async function refresh(){
 if(fetching||!csrf||uninstalling)return;fetching=true;
 try{projects=await request('/api/projects');for(const p of projects){const old=observedOperations.get(p.id),op=p.operation;if(op?.error&&old?.error!==op.error)toast(p.name+': '+op.error,true);else if(old?.pending&&op&&!op.pending&&!op.error)toast(p.name+': '+op.action+' completed');observedOperations.set(p.id,op?{...op}:null);}recordTelemetry();render();$('updated').textContent='Updated '+new Date().toLocaleTimeString([],{hour:'2-digit',minute:'2-digit',second:'2-digit'});}
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
 const progressButton=event.target.closest('[data-progress-id]');if(progressButton){openProjectProgress(progressButton.dataset.progressId,progressButton.dataset.progressAction);return;}
 const taskButton=event.target.closest('[data-task-id]');if(taskButton){const task=await request('/api/tasks/'+taskButton.dataset.taskId);openProgress(task);watchTask(task.id);return;}
 const remove=event.target.closest('[data-remove]');if(remove){openRemove(remove.dataset.remove);return;}
 const button=event.target.closest('[data-action]');if(!button)return;
 const id=button.dataset.id,action=button.dataset.action;
 if(localBusy.has(id))return;
 localBusy.add(id);render();message(action==='stop'?'Stopping project…':action==='restart'?'Restarting project…':'Starting project…');
 try{await post(`/api/projects/${id}/${action}`,{});openProjectProgress(id,action);}catch(e){message(e.message,true);}finally{localBusy.delete(id);await refresh();}
});
$('refresh').addEventListener('click',()=>{message('');refresh();});
$('project-search').addEventListener('input',render);$('project-status').addEventListener('change',render);
$('monitor-attention').addEventListener('click',()=>{$('project-status').value='attention';render();$('project-search').focus();});
$('clear-filters').addEventListener('click',()=>{$('project-search').value='';$('project-status').value='all';render();$('project-search').focus();});
$('project-switcher').addEventListener('change',()=>{location.hash=`project/${$('project-switcher').value}/${detailTab}`;});
$('log-source').addEventListener('change',loadLogs);$('refresh-logs').addEventListener('click',loadLogs);
$('log-search').addEventListener('input',()=>{clearTimeout(searchTimer);searchTimer=setTimeout(loadLogs,250);});
document.querySelectorAll('[data-settings-group]').forEach(button=>button.addEventListener('click',()=>{selectSettingsGroup(button.dataset.settingsGroup);history.replaceState(null,'',`#project/${toolProject}/configuration/${settingsGroup}`);lastRoute=location.hash;}));
$('settings-form').addEventListener('invalid',event=>{const section=event.target.closest('[data-setting-section]');if(section)selectSettingsGroup(section.dataset.settingSection);},true);
$('settings-form').addEventListener('input',()=>{settingsDirty=true;$('settings-save-state').textContent='Unsaved changes';toolNotice('');});
$('settings-form').addEventListener('change',()=>{settingsDirty=true;$('settings-save-state').textContent='Unsaved changes';toolNotice('');});
$('environment-form').addEventListener('input',()=>{environmentDirty=true;toolNotice('');});
document.querySelectorAll('[data-command]').forEach(button=>button.addEventListener('click',()=>{$('action-kind').value=button.dataset.tool||'artisan';$('action-command').value=button.dataset.command;$('action-args').value=button.dataset.args||'';$('schema-confirm').checked=false;LaraControls.sync();$('action-command').focus();}));
window.addEventListener('beforeunload',event=>{if(hasDraft()||savingSettings||savingEnvironment||applicationSaving||uninstalling){event.preventDefault();event.returnValue='';}});
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
function toolNotice(text,error=false){toast(text,error);}
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
  for(const key of ['vite','queue','scheduler','redis','mailpit'])f['service_'+key].checked=!!c.services?.[key];f.service_auto_detect.checked=c.services?.auto_detect!==false;
  f.port.value=c.port||'';f.database.value=c.database||'auto';f.https.value=c.https==null?'':String(c.https);f.phpmyadmin.value=c.phpmyadmin==null?'':String(c.phpmyadmin);
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
 c.services={...c.services,auto_detect:f.service_auto_detect.checked};for(const key of ['vite','queue','scheduler','redis','mailpit'])c.services[key]=f['service_'+key].checked;
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
$('action-form').addEventListener('submit',async event=>{event.preventDefault();if(event.target.dataset.pending)return;event.target.dataset.pending='true';setFormEnabled('action-form',false);try{const args=[$('action-command').value.trim(),...argumentsList($('action-args').value)];const task=await post(`/api/projects/${toolProject}/tasks`,{kind:$('action-kind').value,args,confirm_schema:$('schema-confirm').checked});$('schema-confirm').checked=false;openProgress(task);watchTask(task.id);}catch(e){toolNotice(e.message,true);}finally{delete event.target.dataset.pending;setFormEnabled('action-form',true);}});
async function watchTask(id,reveal=true){
 currentTask=id;try{sessionStorage.setItem('laraserve-task',id);}catch{}clearTimeout(taskTimer);if(reveal)taskDismissed=false;$('task-panel').hidden=taskDismissed||currentView!=='activity';$('activity-empty').hidden=true;
 try{const task=await request(`/api/tasks/${id}`);if(currentTask!==id)return;
  $('task-title').textContent=(task.project_name?task.project_name+' · ':'')+(task.label||task.kind);$('task-status').textContent=task.status+(task.exit_code>=0?' · exit '+task.exit_code:'')+(task.error?' · '+task.error:'');
  const output=$('task-output'),atBottom=output.scrollHeight-output.scrollTop-output.clientHeight<35;output.textContent=task.output||'Waiting for output…';if(atBottom)output.scrollTop=output.scrollHeight;
  const running=['queued','running','recovering'].includes(task.status);$('activity-dot').hidden=!running;$('cancel-task').disabled=!running||task.status==='recovering';
  updateProgress(task,running);
  if(running)taskTimer=setTimeout(()=>watchTask(id,false),700);else{await refresh();if(currentView==='activity')loadTaskHistory();if(currentView==='detail'&&detailTab==='actions')loadTaskHistory(toolProject);}
 }catch(e){$('task-status').textContent=e.message;}
}
$('cancel-task').addEventListener('click',async()=>{if(!currentTask)return;try{await post(`/api/tasks/${currentTask}/cancel`,{});watchTask(currentTask);}catch(e){message(e.message,true);}});
$('create-form').addEventListener('submit',async event=>{event.preventDefault();if(event.target.dataset.pending)return;event.target.dataset.pending='true';setFormEnabled('create-form',false);try{const task=await post('/api/create',{path:$('create-path').value.trim(),starter:$('create-starter').value,database:$('create-database').value,domain:$('create-domain').value.trim().toLowerCase(),port:$('create-port').value?Number($('create-port').value):0,https:$('create-https').checked,vite:$('create-vite').checked,migrate:$('create-migrate').checked});openProgress(task);watchTask(task.id);}catch(e){message(e.message,true);}finally{delete event.target.dataset.pending;setFormEnabled('create-form',true);}});
function runtimeVersions(){$('runtime-version').innerHTML=[...(catalog[$('runtime-kind').value]||[])].reverse().map(v=>`<option value="${escape(v)}">${escape(v)}</option>`).join('');LaraControls.sync();}
async function loadRuntimes(){try{const data=await request('/api/runtimes');$('runtime-list').innerHTML=data.installed.length?data.installed.map(entry=>`<div class="runtime-entry"><div><strong>${escape(entry.kind?({php:'PHP',mysql:'MySQL',node:'Node.js'}[entry.kind]||entry.kind)+' '+entry.version:entry.key)}</strong>${entry.npm?`<span class="badge">npm ${escape(entry.npm)}</span>`:''}<p>${bytes(entry.bytes)} · ${escape(entry.reason||'Unreferenced published cache')}</p></div>${entry.protected?'':`<button class="button" data-remove-cache="${escape(entry.key)}">Remove</button>`}</div>`).join(''):'<p>No published runtimes in the cache yet.</p>';}catch(e){message(e.message,true);}}
$('runtime-kind').addEventListener('change',runtimeVersions);
$('refresh-runtimes').addEventListener('click',loadRuntimes);
$('runtime-install').addEventListener('submit',async event=>{event.preventDefault();if(event.target.dataset.pending)return;event.target.dataset.pending='true';setFormEnabled('runtime-install',false);try{const task=await post('/api/runtimes/install',{kind:$('runtime-kind').value,version:$('runtime-version').value});openProgress(task);watchTask(task.id);}catch(e){message(e.message,true);}finally{delete event.target.dataset.pending;setFormEnabled('runtime-install',true);}});
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
// Folder browsing only lists local directories; selecting a destination creates nothing.
let folderTarget='',folderListing,folderRequest,folderSequence=0;
function folderError(text){$('folder-error').textContent=text;$('folder-error').hidden=!text;}
function selectedFolder(){
 if(!folderListing)return '';
 const name=folderTarget==='create-path'?$('folder-app-name').value.trim():'';
 if(name&&(/[\\/\x00-\x1f]/.test(name)||name==='.'||name==='..'))return '';
 return name?folderListing.path+(folderListing.path.endsWith(folderListing.separator)?'':folderListing.separator)+name:folderListing.path;
}
function updateFolderSelection(){
 const path=selectedFolder();$('folder-selected').textContent=path||'Enter a single folder name, without slashes.';$('folder-choose').disabled=!path;
}
function renderFolders(){
 if(!folderListing)return;
 const query=$('folder-search').value.toLowerCase(),shown=folderListing.folders.filter(folder=>folder.name.toLowerCase().includes(query));
 $('folder-list').innerHTML=shown.length?shown.map(folder=>`<button type="button" class="folder-entry" data-folder-path="${escape(folder.path)}"><span class="folder-icon" aria-hidden="true">▱</span><span>${escape(folder.name)}</span><span class="folder-arrow" aria-hidden="true">›</span></button>`).join(''):`<div class="folder-empty">${query?'No matching folders.':'No subfolders here. You can select this folder.'}</div>`;
 $('folder-note').textContent=folderListing.truncated?'Showing a limited set of folders. Enter a full path to reach another folder.':`${shown.length} ${shown.length===1?'folder':'folders'} · only folder names are listed`;
}
async function loadFolder(path){
 if(folderRequest)folderRequest.abort();
 const controller=new AbortController(),sequence=++folderSequence;folderRequest=controller;
 folderError('');$('folder-list').textContent='Opening folder…';$('folder-choose').disabled=true;
 try{
  const data=await request('/api/folders',{method:'POST',headers:{'Content-Type':'application/json','X-CSRF-Token':csrf},body:JSON.stringify({path,hidden:$('folder-hidden').checked}),signal:controller.signal});
  if(sequence!==folderSequence||!$('folder-dialog').open)return;
  folderListing=data;$('folder-location').value=data.path;$('folder-search').value='';$('folder-up').disabled=data.path===data.parent;
  renderFolders();updateFolderSelection();
 }catch(e){if(e.name!=='AbortError'&&sequence===folderSequence){$('folder-list').textContent='Could not open this folder. Enter another path or choose Home.';folderError(e.message);}}
}
document.querySelectorAll('[data-browse]').forEach(button=>button.addEventListener('click',()=>{
 folderTarget=button.dataset.browse;folderListing=undefined;folderError('');$('folder-search').value='';$('folder-selected').textContent='Choose a folder';
 const creating=folderTarget==='create-path';
 $('folder-title').textContent=creating?'Choose where to create your app':'Choose an existing Laravel app';
 $('folder-description').textContent=creating?'Select a parent folder, then give your new app a folder name.':'Select the source folder containing artisan and composer.json.';
 $('folder-new-name').hidden=!creating;
 let initial=$(folderTarget).value.trim();
 if(creating&&initial){const trimmed=initial.replace(/[\\/]$/,'');const separator=Math.max(trimmed.lastIndexOf('/'),trimmed.lastIndexOf('\\'));const name=trimmed.slice(separator+1);if(name&&separator>=0){$('folder-app-name').value=name;initial=trimmed.slice(0,separator+1);}}
 if(creating&&!$(folderTarget).value.trim())$('folder-app-name').value='my-app';
 LaraControls.close();$('folder-dialog').showModal();loadFolder(initial);
}));
$('folder-location-form').addEventListener('submit',event=>{event.preventDefault();loadFolder($('folder-location').value.trim());});
$('folder-list').addEventListener('click',event=>{const button=event.target.closest('[data-folder-path]');if(button)loadFolder(button.dataset.folderPath);});
$('folder-search').addEventListener('input',renderFolders);
$('folder-app-name').addEventListener('input',updateFolderSelection);
$('folder-hidden').addEventListener('change',()=>loadFolder(folderListing?.path||$('folder-location').value.trim()));
$('folder-up').addEventListener('click',()=>{if(folderListing)loadFolder(folderListing.parent);});
$('folder-home').addEventListener('click',()=>loadFolder(''));
for(const id of ['folder-close','folder-cancel'])$(id).addEventListener('click',()=>$('folder-dialog').close());
$('folder-dialog').addEventListener('close',()=>{if(folderRequest)folderRequest.abort();folderSequence++;});
$('folder-choose').addEventListener('click',()=>{
 const path=selectedFolder();if(!path)return;
 $(folderTarget).value=path;$(folderTarget).dispatchEvent(new Event('input',{bubbles:true}));$('folder-dialog').close();$(folderTarget).focus();
});
$('add-app-form').addEventListener('submit',async event=>{
 event.preventDefault();if(event.target.dataset.pending)return;
 event.target.dataset.pending='true';setFormEnabled('add-app-form',false);$('add-app-notice').hidden=true;
 try{
  const project=await post('/api/projects/add',{path:$('existing-path').value.trim()});
  await refresh();toast("App added. Configure its connection and services.");location.hash=`project/${project.id}/configuration/connection`;
 }catch(e){toast(e.message,true);}
 finally{delete event.target.dataset.pending;setFormEnabled('add-app-form',true);}
});

let applicationDocument,applicationInfo,applicationDirty=false,applicationSaving=false,uninstalling=false,removePending=false,removeProjectId='';
const startupBusy=new Set();
function appNotice(text,error=false){toast(text,error);}
function showUpdate(result){
 $('app-version').textContent=result.current||applicationInfo?.version||'';
 $('latest-version').textContent=result.latest?'Latest published '+result.latest:'';
 $('update-message').textContent=result.message;$('update-checked').textContent=result.checked?'Last checked '+new Date(result.checked).toLocaleString():'';
}
async function loadApplication(){
 appNotice('Loading preferences…');setFormEnabled('application-form',false);
 try{
  applicationInfo=await request('/api/application');applicationDocument=applicationInfo.settings;const c=applicationDocument.config;
  $('app-login').checked=c.launch_at_login;$('app-open').checked=c.open_dashboard;$('app-updates').checked=c.check_updates;
  $('app-platform').textContent=applicationInfo.platform;$('app-home').textContent=applicationInfo.home;$('app-executable').textContent=applicationInfo.executable;$('app-login-path').textContent=applicationInfo.login_path||'Not available on this platform';
  $('uninstalled-executable').textContent=applicationInfo.executable;
  showUpdate(applicationInfo.updates);applicationDirty=false;$('application-save-state').textContent='Startup options apply on your next launch or login.';setFormEnabled('application-form',true);$('app-login').disabled=!applicationInfo.login_supported;appNotice('');
 }catch(e){appNotice(e.message,true);}
}
$('application-form').addEventListener('change',()=>{applicationDirty=true;$('application-save-state').textContent='Unsaved preferences';appNotice('');});
$('application-form').addEventListener('submit',async event=>{
 event.preventDefault();if(!applicationDocument||applicationSaving)return;
 applicationSaving=true;setFormEnabled('application-form',false);
 try{
  applicationDocument=await post('/api/application',{revision:applicationDocument.revision,config:{launch_at_login:$('app-login').checked,open_dashboard:$('app-open').checked,check_updates:$('app-updates').checked}});
  applicationDirty=false;$('application-save-state').textContent='Preferences saved';appNotice('Saved. Selected apps start on the next LaraServe launch; login changes apply when you next sign in.');
 }catch(e){appNotice(e.message,true);}finally{applicationSaving=false;setFormEnabled('application-form',true);}
});
$('reload-application').addEventListener('click',()=>{if(!applicationDirty||confirm('Discard your unsaved preferences and reload?'))loadApplication();});
$('check-updates').addEventListener('click',async()=>{
 const button=$('check-updates');button.disabled=true;button.textContent='Checking…';
 try{showUpdate(await post('/api/application/updates',{}));toast($('update-message').textContent);}catch(e){$('update-message').textContent=e.message;toast(e.message,true);}finally{button.disabled=false;button.textContent='↻ Check for updates';}
});
$('project-startup').addEventListener('change',async()=>{
 const id=toolProject,enabled=$('project-startup').checked;if(startupBusy.has(id))return;
 startupBusy.add(id);$('project-startup').disabled=true;
 try{await post(`/api/projects/${id}/startup`,{enabled});const p=projects.find(p=>p.id===id);if(p)p.startup=enabled;render();toast(enabled?'Project will run when LaraServe starts.':'Project startup disabled.');}
 catch(e){toolNotice(e.message,true);}finally{startupBusy.delete(id);await refresh();}
});
function openRemove(id){
 const p=projects.find(p=>p.id===id);if(!p)return;removeProjectId=id;
 $('remove-title').textContent='Remove '+p.name+'?';$('remove-description').textContent='Remove this app from your LaraServe workspace.'+((id===toolProject&&hasDraft())?' Unsaved edits for this project will be discarded.':'');$('remove-path').textContent=p.source;$('remove-error').hidden=true;$('confirm-remove').disabled=false;$('confirm-remove').textContent='Remove project';$('remove-dialog').showModal();
}
$('remove-current-project').addEventListener('click',()=>openRemove(toolProject));
$('cancel-remove').addEventListener('click',()=>{if(!removePending)$('remove-dialog').close();});
$('remove-dialog').addEventListener('cancel',event=>{if(removePending)event.preventDefault();});
$('confirm-remove').addEventListener('click',async()=>{
 if(removePending)return;const id=removeProjectId;removePending=true;localBusy.add(id);render();$('confirm-remove').disabled=true;$('cancel-remove').disabled=true;$('confirm-remove').textContent='Stopping & removing…';
 function finish(error){removePending=false;localBusy.delete(id);$('cancel-remove').disabled=false;if(error){$('remove-error').textContent=error;$('remove-error').hidden=false;$('confirm-remove').disabled=false;$('confirm-remove').textContent='Try again';render();}else{$('remove-dialog').close();toast('Project removed. Its files and private data were kept.');if(toolProject===id){settingsDirty=false;environmentDirty=false;toolProject='';location.hash='projects';}render();}}
 try{
  await post(`/api/projects/${id}/remove`,{confirm:true});
  let attempts=0;
  const poll=async()=>{try{await refresh();const p=projects.find(p=>p.id===id);if(!p){finish();return;}if(p.operation?.error&&!p.operation.pending){finish(p.operation.error);return;}if(++attempts>180){finish('Removal is taking longer than expected. Check the project status before retrying.');return;}setTimeout(poll,700);}catch(e){finish(e.message);}};
  poll();
 }catch(e){finish(e.message);}
});
function validateUninstall(){
 const active=projects.filter(p=>['running','starting','stopping','unresponsive'].includes(p.status)).length;
 $('uninstall-blocker').textContent=active?`Stop ${active} active ${active===1?'project':'projects'} before uninstalling.`:'All projects are stopped. Finish any running tasks before proceeding.';
 $('confirm-uninstall').disabled=uninstalling||active>0||!$('uninstall-data').checked||$('uninstall-confirm').value!=='UNINSTALL';
}
$('open-uninstall').addEventListener('click',()=>{$('uninstall-error').hidden=true;$('uninstall-confirm').value='';$('uninstall-data').checked=false;validateUninstall();$('uninstall-dialog').showModal();});
$('uninstall-form').addEventListener('input',validateUninstall);
$('cancel-uninstall').addEventListener('click',()=>{if(!uninstalling)$('uninstall-dialog').close();});
$('uninstall-dialog').addEventListener('cancel',event=>{if(uninstalling)event.preventDefault();});
$('uninstall-form').addEventListener('submit',async event=>{
 event.preventDefault();validateUninstall();if($('confirm-uninstall').disabled)return;
 uninstalling=true;$('confirm-uninstall').disabled=true;$('cancel-uninstall').disabled=true;$('confirm-uninstall').textContent='Uninstalling…';
 try{
  await post('/api/application/uninstall',{confirm:$('uninstall-confirm').value,delete_managed_data:$('uninstall-data').checked});
  const poll=async()=>{try{
   const result=await request('/api/application/uninstall-status');
   if(result.pending){setTimeout(poll,500);return;}
   if(result.complete){csrf='';clearTimeout(taskTimer);$('uninstall-dialog').close();$('workspace').hidden=true;$('locked').hidden=true;$('uninstalled').hidden=false;return;}
   throw new Error(result.error||'Uninstall did not complete.');
  }catch(e){uninstalling=false;$('uninstall-error').textContent=e.message;$('uninstall-error').hidden=false;$('cancel-uninstall').disabled=false;$('confirm-uninstall').textContent='Uninstall';validateUninstall();}};poll();
 }catch(e){uninstalling=false;$('uninstall-error').textContent=e.message;$('uninstall-error').hidden=false;$('cancel-uninstall').disabled=false;$('confirm-uninstall').textContent='Uninstall';validateUninstall();}
});

LaraControls.enhance();
connect();


let progressTask='',progressProject='',progressComplete=false,projectProgressTimer;
function resetProgress(title){clearTimeout(projectProgressTimer);$('progress-title').textContent=title;$('progress-status').textContent='Preparing…';$('progress-output').textContent='Waiting for output…';$('progress-indicator').hidden=false;$('progress-project').hidden=true;$('progress-cancel').hidden=true;$('progress-steps').innerHTML='';$('progress-summary').textContent='';$('progress-error').hidden=true;$('progress-log').open=false;if(!$('progress-dialog').open)$('progress-dialog').showModal();}
function renderProgress(progress,running,error=''){
 const steps=(progress?.steps||[]).map(s=>!running&&(error||progress?.error)?(s.status==='running'?{...s,status:'failed',detail:error}:s.status==='pending'?{...s,status:'blocked',detail:'Not run'}:s):s),completed=steps.filter(s=>['completed','skipped'].includes(s.status)).length;
 $('progress-summary').textContent=steps.length?completed+' of '+steps.length+' steps complete':running?'Preparing task…':'Task finished';
 updateHTML($('progress-steps'),steps.map(s=>`<li class="step-${escape(s.status)}"><span class="step-symbol" aria-hidden="true">${s.status==='completed'?'✓':s.status==='skipped'?'–':s.status==='failed'?'!':s.status==='running'?'◌':'○'}</span><div><strong>${escape(s.name)}</strong><small>${escape((s.status==='failed'?'Could not complete this step':s.detail)||({completed:'Completed',running:'In progress…',pending:'Waiting',skipped:'Not needed',failed:'Failed'}[s.status]||''))}</small></div></li>`).join(''));
 $('progress-indicator').hidden=!running;$('progress-error').textContent=error||progress?.error||'';$('progress-error').hidden=!$('progress-error').textContent;
}
function openProgress(task){progressTask=task.id;progressProject='';progressComplete=false;resetProgress((task.project_name?task.project_name+' · ':'')+(task.label||(task.kind==='create'?'Create Laravel app':'Run '+task.kind)));renderProgress(task.progress,true);}
function updateProgress(task,running){if(progressTask!==task.id)return;const output=$('progress-output');const bottom=output.scrollHeight-output.scrollTop-output.clientHeight<50;output.textContent=task.output||'Waiting for output…';if(bottom)output.scrollTop=output.scrollHeight;const active=task.progress?.steps?.find(s=>s.status==='running');$('progress-status').textContent=running?(active?.name||'Preparing…'):task.exit_code===0?'Completed successfully':task.status==='cancelled'?'Task cancelled':'Task failed';renderProgress(task.progress,running,task.error||'');$('progress-cancel').hidden=!running;$('progress-cancel').disabled=task.status==='recovering';if(!running&&!progressComplete){progressComplete=true;toast(task.exit_code===0?(task.label||'Task')+' completed':(task.error||'Task failed.'),task.exit_code!==0);if(task.kind==='create'&&task.exit_code===0&&task.project_id){progressProject=task.project_id;$('progress-project').hidden=false;refresh().then(()=>{if(progressTask===task.id){$('progress-dialog').close();location.hash='#project/'+progressProject+'/configuration/connection';}});}}}
async function openProjectProgress(id,action){progressTask='';progressProject=id;const project=projects.find(p=>p.id===id);resetProgress((project?.name||'Project')+' · '+({start:'Start project',restart:'Restart project',stop:'Stop project'}[action]||action));const poll=async()=>{if(progressProject!==id||!$('progress-dialog').open)return;try{const data=await request('/api/projects/'+id+'/progress');if(progressProject!==id)return;const running=!!data.operation.pending,error=data.operation.error||data.progress.error||'';const active=data.progress.steps?.find(s=>s.status==='running');$('progress-status').textContent=running?(active?.name||'Preparing…'):error?'Could not '+action+' project':action==='stop'?'Project stopped':'Project is running';$('progress-output').textContent=data.output||'Waiting for output…';renderProgress(data.progress,running,error);if(running)projectProgressTimer=setTimeout(poll,650);else await refresh();}catch(e){$('progress-error').textContent=e.message;$('progress-error').hidden=false;$('progress-indicator').hidden=true;}};poll();}
$('progress-close').addEventListener('click',()=>$('progress-dialog').close());
$('progress-dialog').addEventListener('close',()=>clearTimeout(projectProgressTimer));
$('progress-project').addEventListener('click',()=>{$('progress-dialog').close();location.hash='#project/'+progressProject+'/configuration/connection';});
$('create-starter').addEventListener('change',()=>{$('create-vite').checked=$('create-starter').value!=='none';});
async function loadTaskHistory(id=''){const target=$(id?'project-task-history':'activity-history');try{const tasks=await request(id?'/api/projects/'+id+'/tasks':'/api/task-history');if(id&&id!==toolProject)return;updateHTML(target,tasks.length?tasks.map(t=>`<button class="task-history-row" data-task-id="${escape(t.id)}"><span><strong>${escape(t.label||t.kind)}</strong><small>${escape(t.project_name|| (t.project_id?projects.find(p=>p.id===t.project_id)?.name||'Project':'Workspace'))} · ${escape(new Date(t.started_utc).toLocaleString())}</small></span><span class="badge ${escape(t.status)}">${escape(t.status)}</span><span aria-hidden="true">↗</span></button>`).join(''):'<p class="muted">No tasks yet. Each command gets its own named log.</p>');}catch(e){target.textContent=e.message;}}

$('progress-cancel').addEventListener('click',async()=>{if(!progressTask)return;$('progress-cancel').disabled=true;try{await post('/api/tasks/'+progressTask+'/cancel',{});watchTask(progressTask);}catch(e){toast(e.message,true);$('progress-cancel').disabled=false;}});

document.addEventListener('click',async event=>{const button=event.target.closest('[data-copy-url]');if(!button)return;try{await navigator.clipboard.writeText(button.dataset.copyUrl);toast('Application URL copied.');}catch{toast('Could not copy the URL. Select it from the project overview and copy it manually.',true);}});
