'use strict';
window.LaraNotifications=(()=>{
 const key='laraserve-notifications-v1',limit=50;
 const get=id=>document.getElementById(id);
 let entries=[];
 try{const saved=JSON.parse(localStorage.getItem(key)||'[]');if(Array.isArray(saved))entries=saved.filter(n=>n&&typeof n.text==='string'&&Number.isFinite(n.time)&&Math.abs(n.time)<=8640000000000000).slice(0,limit);}catch{}
 function save(){try{localStorage.setItem(key,JSON.stringify(entries));}catch{}render();}
 function render(){
  const unread=entries.filter(n=>!n.read).length;
  get('notifications-count').textContent=unread;
  get('notifications-count').hidden=!unread;
  get('notifications-open').setAttribute('aria-label',`Notifications, ${unread} unread`);
  get('notifications-clear').disabled=!entries.length;
  const list=get('notifications-list');list.replaceChildren();
  if(!entries.length){const empty=document.createElement('li');empty.className='notifications-empty';empty.textContent='You’re all caught up. New updates will appear here.';list.append(empty);return;}
  for(const n of entries){
   const item=document.createElement('li');item.className='notification-entry'+(n.error?' notification-error':'');
   const label=document.createElement('strong');label.textContent=n.error?'Needs attention':'Update';
   const text=document.createElement('p');text.textContent=n.text;
   const time=document.createElement('time');time.dateTime=new Date(n.time).toISOString();time.textContent=new Date(n.time).toLocaleString();
   item.append(label,text,time);list.append(item);
  }
 }
 function show(text,error=false){
  if(!text)return;
  text=String(text);const stack=get('toast-stack');
  if([...stack.children].some(item=>item.dataset.message===text))return;
  entries.unshift({text,error:!!error,time:Date.now(),read:get('notifications-dialog').open});entries=entries.slice(0,limit);save();
  while(stack.children.length>=4)stack.firstElementChild.remove();
  const item=document.createElement('div');item.className='toast '+(error?'toast-error':'');item.dataset.message=text;item.setAttribute('role',error?'alert':'status');
  const content=document.createElement('span');content.textContent=text;
  const close=document.createElement('button');close.textContent='×';close.setAttribute('aria-label','Dismiss notification');
  let timer;const pause=()=>clearTimeout(timer);const resume=()=>{pause();timer=setTimeout(()=>item.remove(),error?10000:6500);};
  close.onclick=()=>{pause();item.remove();};
  item.addEventListener('mouseenter',pause);item.addEventListener('mouseleave',()=>{if(!item.contains(document.activeElement))resume();});
  item.addEventListener('focusin',pause);item.addEventListener('focusout',()=>{if(!item.matches(':hover'))resume();});
  item.append(content,close);stack.append(item);resume();
 }
 get('notifications-open').addEventListener('click',()=>{entries.forEach(n=>n.read=true);save();get('notifications-dialog').showModal();});
 get('notifications-close').addEventListener('click',()=>get('notifications-dialog').close());
 get('notifications-clear').addEventListener('click',()=>{entries=[];save();});
 render();return {show};
})();
