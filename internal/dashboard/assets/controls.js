'use strict';
// Keep real form values as the source of truth, and provide one consistent,
// keyboard-accessible dropdown across desktop and mobile browsers.
window.LaraControls = (() => {
 const widgets = new Map();
 let opened, sequence = 0, typed = '', typeTimer;
 function close(focus = false) {
  if (!opened) return;
  const widget = opened;
  widget.popup.remove(); widget.button.setAttribute('aria-expanded', 'false');
  widget.button.removeAttribute('aria-activedescendant'); opened = undefined;
  if (focus) widget.button.focus();
 }
 function options(widget) { return [...widget.select.options].filter(option => !option.hidden); }
 function highlight(widget, index) {
  const items = options(widget);
  if (!items.length) return;
  widget.active = Math.max(0, Math.min(index, items.length - 1));
  [...widget.popup.children].forEach((node, i) => node.classList.toggle('highlighted', i === widget.active));
  const node = widget.popup.children[widget.active];
  if (node) { widget.button.setAttribute('aria-activedescendant', node.id); node.scrollIntoView({block: 'nearest'}); }
 }
 function position(widget) {
  const rect = widget.button.getBoundingClientRect();
  const width = Math.min(Math.max(rect.width, 210), window.innerWidth - 24);
  const below = window.innerHeight - rect.bottom - 12, above = rect.top - 12;
  const upward = below < 180 && above > below;
  widget.popup.style.width = width + 'px';
  widget.popup.style.left = Math.max(12, Math.min(rect.left, window.innerWidth - width - 12)) + 'px';
  widget.popup.style.maxHeight = Math.max(80, Math.min(320, upward ? above : below)) + 'px';
  if (upward) { widget.popup.style.bottom = (window.innerHeight - rect.top + 7) + 'px'; widget.popup.style.top = 'auto'; }
  else { widget.popup.style.top = (rect.bottom + 7) + 'px'; widget.popup.style.bottom = 'auto'; }
 }
 function choose(widget, option) {
  if (option.disabled || widget.select.disabled) return;
  const changed = widget.select.value !== option.value;
  widget.select.value = option.value; close(true); syncOne(widget);
  if (changed) widget.select.dispatchEvent(new Event('change', {bubbles: true}));
 }
 function fillPopup(widget) {
  widget.popup.replaceChildren();
  for (const [index, option] of options(widget).entries()) {
   const item = document.createElement('div'); item.className = 'select-option';
   item.id = `${widget.id}-option-${index}`; item.setAttribute('role', 'option');
   item.setAttribute('aria-selected', String(option.selected));
   if (option.disabled) item.setAttribute('aria-disabled', 'true');
   const text = document.createElement('span'); text.textContent = option.text;
   const check = document.createElement('span'); check.className = 'option-check'; check.textContent = option.selected ? '✓' : '';
   check.setAttribute('aria-hidden', 'true'); item.append(text, check);
   item.addEventListener('pointermove', () => highlight(widget, index));
   item.addEventListener('click', () => choose(widget, option)); widget.popup.append(item);
  }
  if (!widget.popup.children.length) { const empty = document.createElement('div'); empty.className = 'select-empty'; empty.textContent = 'No options available'; widget.popup.append(empty); }
 }
 function open(widget) {
  if (widget.select.disabled) return;
  close(); opened = widget; widget.button.setAttribute('aria-expanded', 'true');
  fillPopup(widget); document.body.append(widget.popup); position(widget);
  highlight(widget, Math.max(0, options(widget).findIndex(option => option.selected)));
 }
 function syncOne(widget) {
  const selected = widget.select.selectedOptions[0];
  widget.value.textContent = selected?.text || 'Choose an option';
  widget.button.disabled = widget.select.disabled;
  const signature = [...widget.select.options].map(option => [option.text, option.value, option.selected, option.disabled, option.hidden].join('\0')).join('\n');
  if (opened === widget) {
   if (widget.select.disabled) close();
   else if (widget.signature !== signature) { fillPopup(widget); position(widget); highlight(widget, options(widget).findIndex(option => option.selected)); }
  }
  widget.signature = signature;
 }
 function enhance() {
  for (const select of document.querySelectorAll('select:not([data-native-only])')) {
   if (widgets.has(select)) continue;
   const label = select.labels?.[0];
   const clone = label?.cloneNode(true); clone?.querySelectorAll('select, .select-widget').forEach(node => node.remove());
   const name = select.getAttribute('aria-label') || clone?.textContent.trim() || 'Choose an option';
   const id = `dropdown-${++sequence}`;
   const wrapper = document.createElement('span'); wrapper.className = 'select-widget';
   select.before(wrapper); wrapper.append(select); select.hidden = true;
   const button = document.createElement('button'); button.type = 'button'; button.className = 'select-trigger';
   button.setAttribute('role', 'combobox'); button.setAttribute('aria-label', name);
   button.setAttribute('aria-haspopup', 'listbox'); button.setAttribute('aria-expanded', 'false'); button.setAttribute('aria-controls', id);
   const value = document.createElement('span'); value.className = 'select-value'; value.id = id + '-value';
   button.setAttribute('aria-describedby', value.id);
   const chevron = document.createElement('span'); chevron.className = 'select-chevron'; chevron.textContent = '⌄'; chevron.setAttribute('aria-hidden', 'true');
   button.append(value, chevron); wrapper.append(button);
   const popup = document.createElement('div'); popup.id = id; popup.className = 'select-popover'; popup.setAttribute('role', 'listbox'); popup.setAttribute('aria-label', name);
   const widget = {select, wrapper, button, value, popup, id, active: 0}; widgets.set(select, widget);
   button.addEventListener('click', event => { event.preventDefault(); if (opened === widget) close(); else open(widget); });
   button.addEventListener('keydown', event => {
    const items = options(widget);
    if (['ArrowDown', 'ArrowUp', 'Home', 'End'].includes(event.key)) {
     event.preventDefault(); const wasOpen = opened === widget;
     if (!wasOpen) open(widget);
     let index = widget.active;
     if (event.key === 'Home') index = 0;
     else if (event.key === 'End') index = items.length - 1;
     else if (wasOpen) index += event.key === 'ArrowDown' ? 1 : -1;
     index = Math.max(0, Math.min(index, items.length - 1));
     while (items[index]?.disabled && index >= 0 && index < items.length) index += event.key === 'ArrowUp' ? -1 : 1;
     highlight(widget, index);
    } else if (event.key === 'Enter' || event.key === ' ') {
     event.preventDefault(); if (opened !== widget) open(widget); else if (items[widget.active]) choose(widget, items[widget.active]);
    } else if (event.key === 'Escape' && opened === widget) { event.preventDefault(); event.stopPropagation(); close(true); }
    else if (event.key === 'Tab') close();
    else if (event.key.length === 1 && !event.ctrlKey && !event.metaKey && !event.altKey) {
     event.preventDefault(); if (opened !== widget) open(widget);
     clearTimeout(typeTimer); typed += event.key.toLowerCase(); typeTimer = setTimeout(() => { typed = ''; }, 600);
     const index = items.findIndex(option => !option.disabled && option.text.toLowerCase().startsWith(typed));
     if (index >= 0) highlight(widget, index);
    }
   });
   select.addEventListener('change', () => syncOne(widget));
   new MutationObserver(() => syncOne(widget)).observe(select, {childList: true, subtree: true, attributes: true, characterData: true});
   syncOne(widget);
  }
 }
 document.addEventListener('pointerdown', event => { if (opened && !opened.wrapper.contains(event.target) && !opened.popup.contains(event.target)) close(); });
 document.addEventListener('scroll', event => { if (opened && !opened.popup.contains(event.target)) close(); }, true);
 window.addEventListener('resize', () => close());
 return {enhance, sync() { for (const widget of widgets.values()) syncOne(widget); }, close};
})();
