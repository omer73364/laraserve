'use strict';
const test=require('node:test');
const assert=require('node:assert/strict');
const fs=require('node:fs');
const vm=require('node:vm');
const path=require('node:path');
const assets=path.join(__dirname,'../../internal/dashboard/assets');
const app=fs.readFileSync(path.join(assets,'app.js'),'utf8');

function loaderHarness(){
 const nodes=new Map(),pending=[],notices=[];
 const node=id=>{if(!nodes.has(id))nodes.set(id,{value:'',textContent:'',innerHTML:'',reset(){},replaceChildren(){},elements:new Proxy({},{get:(o,k)=>o[k]||(o[k]={value:'',checked:false})})});return nodes.get(id);};
 const context=vm.createContext({$:node,request:url=>new Promise((resolve,reject)=>pending.push({url,resolve,reject})),setFormEnabled(){},renderDetail(){},toolNotice:text=>{if(text)notices.push(text);},escape:String,projects:[{id:'a'},{id:'b'}]});
 const settings=app.slice(app.indexOf('async function openTools('),app.indexOf('async function loadEnvironment('));
 const environment=app.slice(app.indexOf('async function loadEnvironment('),app.indexOf("$('settings-form').addEventListener('submit'"));
 vm.runInContext(`let toolProject='',settingsSequence=0,environmentSequence=0,settingsLoading=false,settingsDirty=false,configDocument,environmentDocument,environmentLoaded=false,environmentLoading=false,environmentDirty=false;const changedVariables=new Map();const phpFields=[];${settings}\n${environment}\nthis.open=openTools;this.environment=loadEnvironment;this.setProject=id=>toolProject=id;this.state=()=>({configDocument,environmentDocument,settingsLoading,environmentLoading});`,context);
 return {context,pending,notices};
}

test('a slow settings response cannot replace a newer response for the same project',async()=>{
 const {context,pending,notices}=loaderHarness();
 const first=context.open('a'),middle=context.open('b'),latest=context.open('a');
 pending[2].resolve({revision:'new',config:{domain:'new.test'}});await latest;
 pending[1].resolve({revision:'other',config:{domain:'other.test'}});await middle;
 pending[0].reject(new Error('obsolete request failed'));await first;
 assert.equal(context.state().configDocument.revision,'new');
 assert.equal(context.state().settingsLoading,false);
 assert.deepEqual(notices,[]);
});

test('masked environment responses cannot overwrite a more recent project load',async()=>{
 const {context,pending}=loaderHarness();context.setProject('a');
 const first=context.environment();context.setProject('b');const middle=context.environment();context.setProject('a');const latest=context.environment();
 pending[2].resolve({revision:'new',variables:[]});await latest;
 pending[1].resolve({revision:'other',variables:[]});await middle;
 pending[0].resolve({revision:'old',variables:[]});await first;
 assert.equal(context.state().environmentDocument.revision,'new');
 assert.equal(context.state().environmentLoading,false);
});

class Element {
 constructor(){this.children=[];this.dataset={};this.listeners={};this.open=false;}
 append(...items){for(const item of items){item.parent=this;this.children.push(item);}}
 replaceChildren(){this.children=[];}
 setAttribute(){}
 addEventListener(name,fn){this.listeners[name]=fn;}
 remove(){this.parent.children=this.parent.children.filter(item=>item!==this);}
 get firstElementChild(){return this.children[0];}
 contains(){return false;}
 matches(){return false;}
 showModal(){this.open=true;}
 close(){this.open=false;}
}
function notificationHarness(storage=new Map()){
 const nodes=new Map(),timers=new Map();let next=0;
 const get=id=>{if(!nodes.has(id))nodes.set(id,new Element());return nodes.get(id);};
 const context=vm.createContext({window:{},document:{getElementById:get,createElement:()=>new Element()},localStorage:{getItem:key=>storage.get(key),setItem:(key,value)=>storage.set(key,value)},setTimeout:(fn,delay)=>{const id=++next;timers.set(id,{fn,delay});return id;},clearTimeout:id=>timers.delete(id)});
 vm.runInContext(fs.readFileSync(path.join(assets,'notifications.js'),'utf8'),context);
 return {api:context.window.LaraNotifications,get,timers,storage,entries:()=>JSON.parse(storage.get('laraserve-notifications-v1')||'[]')};
}
test('transient progress does not clutter saved notifications; all toasts expire',()=>{
 const h=notificationHarness();h.api.show('Starting…',false,{persist:false});assert.equal(h.entries().length,0);
 h.api.show('Saved');h.api.show('Failed',true);assert.deepEqual([...h.timers.values()].map(t=>t.delay),[6500,6500,10000]);
 for(const timer of h.timers.values())timer.fn();assert.equal(h.get('toast-stack').children.length,0);
 h.api.show('Failed',true);assert.equal(h.entries().length,2,'repeated polling errors should not add another history item');
 const restored=notificationHarness(h.storage);assert.equal(restored.entries().length,2);restored.get('notifications-open').listeners.click();assert.equal(restored.get('notifications-count').hidden,true);
});
test('notification history and message sizes stay bounded and can be cleared',()=>{
 const h=notificationHarness();h.api.show('x'.repeat(10000));assert.equal(h.entries()[0].text.length,4000);
 for(let i=0;i<60;i++)h.api.show('Update '+i);assert.equal(h.entries().length,50);h.get('notifications-clear').listeners.click();assert.equal(h.entries().length,0);
});

for(const restart of [false,true])test(`settings restart only when explicitly requested: ${restart}`,async()=>{
 const posts=[],restarts=[],notices=[];let handler;
 const fields=new Proxy({},{get:(obj,key)=>obj[key]||(obj[key]={value:'',checked:false})});fields.database.value='auto';
 const form={elements:fields,addEventListener:(name,fn)=>handler=fn};
 const context=vm.createContext({$:id=>id==='settings-form'?form:{textContent:''},setFormEnabled(){},post:async(url,body)=>{posts.push({url,body});return {revision:'new',config:body.config};},restartSavedProject:async id=>restarts.push(id),renderDetail(){},toolNotice:text=>notices.push(text)});
 const source=app.slice(app.indexOf("$('settings-form').addEventListener('submit'"),app.indexOf("$('environment-fields').addEventListener('input'"));
 vm.runInContext(`let configDocument={revision:'old',config:{}},savingSettings=false,settingsDirty=true,toolProject='a';const phpFields=[];${source}`,context);
 await handler({preventDefault(){},target:form,submitter:{dataset:{restart:String(restart)}}});
 assert.equal(posts.length,1);assert.equal(posts[0].url,'/api/projects/a/settings');assert.equal(posts[0].body.revision,'old');assert.deepEqual(restarts,restart?['a']:[]);
 if(!restart)assert(notices.some(text=>text.includes('Restart the project')));
});

test('expired sessions clear the token so background refresh stops',async()=>{
 const nodes=new Map();const get=id=>{if(!nodes.has(id))nodes.set(id,{hidden:false});return nodes.get(id);};
 const context=vm.createContext({$:get,fetch:async()=>({ok:false,status:401,text:async()=>'Open your dashboard link again'})});
 const source=app.slice(app.indexOf('async function request('),app.indexOf('async function refresh('));
 vm.runInContext(`let csrf='old-token';${source}\nthis.request=request;this.token=()=>csrf;`,context);
 await assert.rejects(context.request('/api/projects'),/Open your dashboard link/);
 assert.equal(context.token(),'');assert.equal(get('locked').hidden,false);assert.equal(get('workspace').hidden,true);
});
