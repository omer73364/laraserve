'use strict';
window.LaraQuickOpen=(()=>{
 const get=id=>document.getElementById(id),dialog=get('quick-open-dialog'),input=get('quick-open-query'),results=get('quick-open-results');
 const pages=[['Projects','projects'],['Runtimes','runtimes'],['Activity','activity'],['Settings','application'],['Create app','new'],['Add existing app','add']].map(([name,route])=>({name,route,kind:'Workspace'}));
 let projects=[],matches=[],active=0,previousFocus;
 function highlight(){
  const options=[...results.children];
  options.forEach((option,index)=>option.setAttribute('aria-selected',String(index===active)));
  if(options[active]){input.setAttribute('aria-activedescendant',options[active].id);options[active].scrollIntoView({block:'nearest'});}else input.removeAttribute('aria-activedescendant');
 }
 function render(){
  const query=input.value.trim().toLocaleLowerCase();
  matches=[...projects,...pages].filter(item=>item.name.toLocaleLowerCase().includes(query)).slice(0,30);active=0;results.replaceChildren();
  matches.forEach((item,index)=>{
   const option=document.createElement('button');option.type='button';option.tabIndex=-1;option.id='quick-open-option-'+index;option.setAttribute('role','option');
   const icon=document.createElement('span');icon.className='quick-open-icon';icon.textContent=item.kind==='Project'?'◇':'↗';icon.setAttribute('aria-hidden','true');
   const text=document.createElement('strong');text.textContent=item.name;
   const kind=document.createElement('small');kind.textContent=item.kind;
   option.append(icon,text,kind);option.addEventListener('click',()=>activate(index));results.append(option);
  });
  get('quick-open-empty').hidden=matches.length!==0;highlight();
 }
 function activate(index){const item=matches[index];if(!item)return;dialog.close();location.hash=item.route;}
 function open(){
  if(document.querySelector('dialog[open]')||get('workspace').hidden)return;
  previousFocus=document.activeElement;input.value='';render();dialog.showModal();input.focus();
 }
 get('quick-open-trigger').addEventListener('click',open);
 get('quick-open-close').addEventListener('click',()=>dialog.close());
 dialog.addEventListener('close',()=>{if(previousFocus?.isConnected)previousFocus.focus();});
 input.addEventListener('input',render);
 input.addEventListener('keydown',event=>{
  if(event.key==='ArrowDown'||event.key==='ArrowUp'){event.preventDefault();if(matches.length)active=(active+(event.key==='ArrowDown'?1:-1)+matches.length)%matches.length;highlight();}
  else if(event.key==='Enter'){event.preventDefault();activate(active);}
 });
 document.addEventListener('keydown',event=>{
  if(get('workspace').hidden)return;
  if((event.ctrlKey||event.metaKey)&&!event.altKey&&event.key.toLowerCase()==='k'){event.preventDefault();open();}
  else if(event.key==='/'&&!event.ctrlKey&&!event.metaKey&&!event.altKey&&!document.querySelector('dialog[open]')&&!event.target.isContentEditable&&!event.target.closest('input,textarea,select')){
   const search=get('project-search');if(search&&search.getClientRects().length){event.preventDefault();search.focus();}
  }
 });
 return {setProjects(items){const next=items.filter(p=>/^[a-f0-9]{12}$/.test(p.id)).map(p=>({name:p.name,route:'project/'+p.id+'/overview',kind:'Project'}));if(JSON.stringify(next)===JSON.stringify(projects))return;projects=next;if(dialog.open)render();}};
})();
