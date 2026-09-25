import {useEffect, useId, useRef, useState, type ReactNode, type CSSProperties} from 'react';
import {DndContext, closestCenter, KeyboardSensor, PointerSensor, useSensor, useSensors, type DragEndEvent} from '@dnd-kit/core';
import {SortableContext, useSortable, verticalListSortingStrategy, sortableKeyboardCoordinates, arrayMove} from '@dnd-kit/sortable';
import {CSS} from '@dnd-kit/utilities';
import {request} from './api';

export function SortList({ids,onOrder,children,busy}:{ids:string[];onOrder:(ids:string[])=>void;children:ReactNode;busy:boolean}) {
  const sensors=useSensors(useSensor(PointerSensor,{activationConstraint:{distance:7}}),useSensor(KeyboardSensor,{coordinateGetter:sortableKeyboardCoordinates,keyboardCodes:{start:['Space'],cancel:['Escape'],end:['Space']}}));
  function end({active,over}:DragEndEvent){if(busy||!over||active.id===over.id)return;const from=ids.indexOf(String(active.id)),to=ids.indexOf(String(over.id));if(from>=0&&to>=0)onOrder(arrayMove(ids,from,to));}
  return <DndContext sensors={sensors} collisionDetection={closestCenter} onDragEnd={end}><SortableContext items={ids} strategy={verticalListSortingStrategy}>{children}</SortableContext></DndContext>;
}
export function SortRow({id,disabled,kind,number,onClick,children,selected=false}:{id:string;disabled:boolean;kind:'scene'|'line';number:number;onClick:()=>void;children:ReactNode;selected?:boolean}) {
  const {attributes,listeners,setNodeRef,setActivatorNodeRef,transform,transition,isDragging}=useSortable({id,disabled});
  const dragged=useRef(false);
  const Element=kind==='line'?'article':'div';
  const style:CSSProperties={transform:CSS.Transform.toString(transform),transition,position:'relative',zIndex:isDragging?2:undefined,opacity:isDragging?.65:1};
  return <Element ref={setNodeRef} style={style} className={`${kind==='line'?'dialogue':'scene-row'} ${selected?'selected':''} ${isDragging?'dragging':''}`} data-sort-id={id}>
    <button {...attributes} {...listeners} ref={setActivatorNodeRef} className={`${kind}-number drag-handle`} disabled={disabled}
      aria-label={kind==='line'?`Edit or move line ${number}`:`Select or move scene ${number}`} title={`Click to ${kind==='line'?'edit':'select'}; drag to reorder. Space then arrow keys to move.`}
      onPointerDown={e=>{dragged.current=false;listeners?.onPointerDown?.(e);}}
      onPointerMove={()=>{if(isDragging)dragged.current=true;}}
      onClick={()=>{if(!isDragging&&!dragged.current)onClick();}}>{String(number).padStart(2,'0')}</button>
    {children}
  </Element>;
}

export function ConfirmDialog({title,children,busy,onCancel,onConfirm}:{title:string;children:ReactNode;busy:boolean;onCancel:()=>void;onConfirm:()=>void}) {
  const ref=useRef<HTMLDialogElement>(null);const titleID=useId();
  useEffect(()=>{ref.current?.showModal();const el=ref.current;return()=>el?.close();},[]);
  return <dialog ref={ref} aria-labelledby={titleID} className="confirm-dialog" onCancel={e=>{e.preventDefault();if(!busy)onCancel();}}>
    <h2 id={titleID}>{title}</h2><div className="dialog-copy">{children}</div><div className="actions"><button className="secondary" autoFocus disabled={busy} onClick={onCancel}>Cancel</button><button className="danger" disabled={busy} onClick={onConfirm}>{busy?'Removing…':'Confirm removal'}</button></div>
  </dialog>;
}
export function DeleteLine({number,busy,onDelete}:{number:number;busy:boolean;onDelete:()=>void}) {
  const [armed,setArmed]=useState(false);
  useEffect(()=>{if(!armed)return;const timer=setTimeout(()=>setArmed(false),8000);return()=>clearTimeout(timer);},[armed]);
  return <div className="line-delete"><button className={armed?'text-button confirm-delete':'text-button'} disabled={busy} aria-label={`${armed?'Confirm delete':'Delete'} line ${number}`} onClick={()=>{if(armed)onDelete();else setArmed(true);}}>{armed?'Confirm':'Delete'}</button>{armed&&<button className="text-button" disabled={busy} onClick={()=>setArmed(false)} aria-label={`Cancel deleting line ${number}`}>Cancel</button>}</div>;
}

const sample=`[script: The lantern in the woods]
[cast: Scout | Hazel]
[cast: Owl | Hannah]

[scene: A light between the trees]
[Scout]
[direction: Quietly, with wonder]
Did you see that little light?

[Owl]
There, just beyond the old oak.

[scene: Following the light]
[Scout]
Then let's find out who is there.`;
type Plan={name:string;cast:{name:string;actor:string}[];scenes:{name:string;lines:{character:string;text:string;direction:string}[]}[];line_count:number};
function newRequestID(){return Array.from(crypto.getRandomValues(new Uint8Array(16)),b=>b.toString(16).padStart(2,'0')).join('');}
export function Importer({busy,save,onClose}:{busy:boolean;save:(path:string,body:unknown)=>Promise<boolean>;onClose:()=>void}) {
  const [text,setText]=useState('');const [plan,setPlan]=useState<Plan>();const [error,setError]=useState('');const [checking,setChecking]=useState(false);const [requestID,setRequestID]=useState(newRequestID);
  const generation=useRef(0);
  function change(value:string){generation.current++;setText(value);setPlan(undefined);setError('');setRequestID(newRequestID());}
  async function preview(){const current=generation.current;setChecking(true);setError('');try{const result=await request<Plan>('/api/import/preview','POST',{text});if(current===generation.current)setPlan(result);}catch(e){if(current===generation.current)setError(e instanceof Error?e.message:'Cannot preview script.');}finally{setChecking(false);}}
  async function load(file:File|undefined){if(!file)return;if(file.size>512*1024){setError('Choose a text file smaller than 512 KB.');return;}try{change(await file.text());}catch{setError('Could not read that file.');}}
  async function submit(){if(await save('/api/import',{text,request_id:requestID}))onClose();}
  return <section className="import-panel card" aria-label="Script importer"><div className="tools-heading"><div><h2>Bring your story in</h2><p>Paste a tagged script or choose a UTF-8 text file. Import creates a new project.</p></div><button className="secondary" disabled={busy||checking} onClick={onClose}>Close importer</button></div>
    <details className="tag-help"><summary>Simple tag guide</summary><p>Tags go on their own lines. A character’s dialogue continues until the next tag. Names are case-sensitive.</p><ul><li><code>[script: Title]</code> — the project title, first.</li><li><code>[cast: Character | Actor]</code> — optional cast entries before scenes; actors with matching names are reused.</li><li><code>[scene: Name]</code> — begin a new scene.</li><li><code>[Character]</code> — begin dialogue; new characters are created unassigned.</li><li><code>[direction: Note]</code> — optional, before dialogue.</li></ul><p>For dialogue that starts with a literal bracket, write <code>\[</code>. Up to 200 scenes, 100 characters, and 2,000 dialogue entries per import.</p></details>
    <div className="import-actions"><label className="file-input">Open text file<input type="file" accept=".txt,.md,text/plain,text/markdown" disabled={busy||checking} onChange={e=>void load(e.target.files?.[0])}/></label><button className="secondary" disabled={busy||checking} onClick={()=>change(sample)}>Use example</button></div>
    <label>Tagged script<textarea aria-label="Tagged script" rows={12} value={text} disabled={busy||checking} onChange={e=>change(e.target.value)} placeholder={sample}/></label>
    {error&&<p className="banner error" role="alert">{error}</p>}
    <div className="actions"><button disabled={busy||checking||!text.trim()} onClick={preview}>{checking?'Checking…':'Preview import'}</button></div>
    {plan&&<div className="import-preview"><h3>{plan.name}</h3><p>{plan.scenes.length} scenes · {plan.cast.length} characters · {plan.line_count} dialogue lines</p><p className="hint">Cast: {plan.cast.map(c=>`${c.name} (${c.actor||'unassigned'})`).join(', ')}. Reading order follows the file; all start times begin at 0 ms.</p><ol>{plan.scenes.map((s,i)=><li key={i}><strong>{s.name}</strong> — {s.lines.length} lines</li>)}</ol><button disabled={busy} onClick={submit}>{busy?'Importing…':'Import as new script'}</button></div>}
  </section>;
}
