import {useEffect,useRef,useState,type FormEvent} from 'react';
import App from './App';
import {ScenePreview} from './ScenePreview';
import {empty,request,type Workspace,type Line} from './api';
import './studio.css';

export type Session={user:{email:string;admin:boolean;actor_id:string;name:string};recording_enabled:boolean};
export type Take={id:string;event_id:string;actor_id:string;revision:number;take_number:number;preferred:boolean;created_at:string;duration_ms:number;mime_type:string;stale:boolean;text:string;direction:string;character_name:string;scene_name:string;project_name:string;actor_name:string};
const errorMessage=(e:unknown)=>e instanceof Error?e.message:'Something went wrong. Please try again.';
export default function Studio(){
 const [session,setSession]=useState<Session>();const [error,setError]=useState('');const [mode,setMode]=useState('');
 useEffect(()=>{request<Session>('/api/session').then(s=>{setSession(s);setMode(s.user.admin?'admin':'actor');}).catch(e=>setError(errorMessage(e)));},[]);
 if(!session)return <div className="studio-gate"><div className="eyebrow">STORYFORGE</div><h1>{error?'Let’s get you signed in.':'Opening your studio…'}</h1>{error&&<><p role="alert">{error}</p><p>Use the secure StoryForge address and your Google account.</p><button onClick={()=>location.reload()}>Try again</button></>}</div>;
 if(mode==='admin'&&session.user.admin)return <App onStudio={()=>setMode('actor')} onReview={()=>setMode('review')}/>;
 if(mode==='review'&&session.user.admin)return <Review onBack={()=>setMode('admin')}/>;
 return <ActorStudio session={session} onAdmin={()=>setMode('admin')}/>;
}

export function ActorEmail({id,name,busy}:{id:string;name:string;busy:boolean}){
 const [email,setEmail]=useState('');const [original,setOriginal]=useState('');const [saving,setSaving]=useState(false);const [message,setMessage]=useState('');
 useEffect(()=>{request<{actor_id:string;email:string}[]>('/api/actor-logins').then(rows=>{const value=rows.find(x=>x.actor_id===id)?.email||'';setEmail(value);setOriginal(value);}).catch(e=>setMessage(errorMessage(e)));},[id]);
 async function submit(e:FormEvent){e.preventDefault();setSaving(true);setMessage('');try{await request(`/api/actors/${id}/login`,'PUT',{email});setOriginal(email);setMessage('Login saved.');}catch(e){setMessage(errorMessage(e));}finally{setSaving(false);}}
 return <form className="actor-email" onSubmit={submit}><label>Google login email<input aria-label={`Login email for ${name}`} type="email" value={email} onChange={e=>setEmail(e.target.value)} placeholder="Their Google account email" disabled={busy||saving}/></label><button className="secondary" disabled={busy||saving||email===original}>Save login</button>{message&&<small role="status">{message}</small>}</form>;
}

function TakeCard({take,onPreferred}:{take:Take;onPreferred:()=>Promise<void>}){
 const [busy,setBusy]=useState(false);const [error,setError]=useState('');
 async function prefer(){setBusy(true);setError('');try{await request(`/api/actor/takes/${take.id}/preferred`,'PUT',{preferred:!take.preferred});await onPreferred();}catch(e){setError(errorMessage(e));}finally{setBusy(false);}}
 return <article className="take-card"><div className="take-heading"><strong>Take {take.take_number}</strong><span>{new Date(take.created_at).toLocaleString()} · {(take.duration_ms/1000).toFixed(1)}s</span>{take.stale&&<span className="old-take">Earlier script version</span>}</div><audio controls preload="none" src={`/api/actor/takes/${take.id}/audio`} aria-label={`Play take ${take.take_number}`} onPlay={e=>{const current=e.currentTarget;document.querySelectorAll('audio').forEach(a=>{if(a!==current)a.pause();});}}/><button className={take.preferred?'preferred':'secondary'} disabled={busy} onClick={prefer}>{take.preferred?'★ Preferred take':'☆ Make preferred'}</button>{take.stale&&<details><summary>Words recorded for this take</summary><p>{take.text}</p>{take.direction&&<p>{take.direction}</p>}</details>}{error&&<p role="alert">{error}</p>}</article>;
}

function Recorder({line,enabled,onSaved,onDirty}:{line:Line;enabled:boolean;onSaved:()=>Promise<void>;onDirty:(dirty:boolean)=>void}){
 const [phase,setPhase]=useState<'idle'|'asking'|'recording'|'preview'|'uploading'|'saved'>('idle');const [blob,setBlob]=useState<Blob>();const [url,setURL]=useState('');const [seconds,setSeconds]=useState(0);const [error,setError]=useState('');
 const media=useRef<MediaRecorder | undefined>(undefined);const stream=useRef<MediaStream | undefined>(undefined);const chunks=useRef<Blob[]>([]);const alive=useRef(true);const uploadID=useRef('');const started=useRef(0);const ticker=useRef<ReturnType<typeof setInterval> | undefined>(undefined);const bytes=useRef(0);
 function stop(){if(media.current?.state==='recording')media.current.stop();stream.current?.getTracks().forEach(t=>t.stop());clearInterval(ticker.current);}
 useEffect(()=>{alive.current=true;return()=>{alive.current=false;stop();};},[]);
 useEffect(()=>{if(!blob){setURL('');return;}const value=URL.createObjectURL(blob);setURL(value);return()=>URL.revokeObjectURL(value);},[blob]);
 useEffect(()=>{onDirty(['asking','recording','preview','uploading'].includes(phase));return()=>onDirty(false);},[phase,onDirty]);
 async function record(){
  if(blob&&phase!=='saved'&&!window.confirm('Try again and discard this unsaved take?'))return;
  setError('');setPhase('asking');
  try{
   if(!window.isSecureContext||!navigator.mediaDevices?.getUserMedia)throw new Error('Open StoryForge using its HTTPS address to use your microphone.');
   if(typeof MediaRecorder==='undefined')throw new Error('This browser cannot record audio. Please open StoryForge in Chrome.');
   const audio=await navigator.mediaDevices.getUserMedia({audio:{channelCount:1,echoCancellation:false,noiseSuppression:false,autoGainControl:false}});
   if(!alive.current){audio.getTracks().forEach(t=>t.stop());return;}stream.current=audio;
   const type=['audio/webm;codecs=opus','audio/mp4','audio/ogg;codecs=opus'].find(t=>MediaRecorder.isTypeSupported(t));
   const recorder=new MediaRecorder(audio,type?{mimeType:type,audioBitsPerSecond:128000}:undefined);media.current=recorder;chunks.current=[];bytes.current=0;
   recorder.ondataavailable=e=>{if(e.data.size){chunks.current.push(e.data);bytes.current+=e.data.size;if(bytes.current>24*1024*1024)stop();}};
   recorder.onerror=()=>{if(alive.current)setError('Recording was interrupted. Listen to what was captured before saving.');stop();};
   recorder.onstop=()=>{clearInterval(ticker.current);audio.getTracks().forEach(t=>t.stop());if(!alive.current)return;const result=new Blob(chunks.current,{type:recorder.mimeType||type||'audio/webm'});if(!result.size){setError('No audio was captured. Check your microphone and try again.');setPhase('idle');return;}setBlob(result);uploadID.current=Array.from(crypto.getRandomValues(new Uint8Array(16)),b=>b.toString(16).padStart(2,'0')).join('');setPhase('preview');};
   audio.getAudioTracks()[0].onended=()=>{if(recorder.state==='recording')stop();};
   document.querySelectorAll('audio').forEach(a=>a.pause());recorder.start(1000);setBlob(undefined);started.current=Date.now();setSeconds(0);setPhase('recording');
   ticker.current=setInterval(()=>{const elapsed=Math.floor((Date.now()-started.current)/1000);if(alive.current)setSeconds(elapsed);if(elapsed>=290)stop();},250);
  }catch(e){stream.current?.getTracks().forEach(t=>t.stop());if(alive.current){setError(e instanceof DOMException&&e.name==='NotAllowedError'?'Microphone access was not allowed. Use the browser’s site settings to allow it, then try again.':errorMessage(e));setPhase(blob?'preview':'idle');}}
 }
 async function save(){if(!blob)return;setPhase('uploading');setError('');try{
  const response=await fetch(`/api/actor/events/${line.id}/takes?revision=${line.revision}`,{method:'POST',headers:{'Content-Type':blob.type,'X-Upload-ID':uploadID.current},body:blob});
  if(!response.headers.get('content-type')?.includes('application/json'))throw new Error('Your login may have expired. Download this take before signing in again.');
  const result=await response.json();if(!response.ok)throw new Error(result.error||'Upload failed. Try Save take again.');
  if(!alive.current)return;setPhase('saved');try{await onSaved();}catch{setError('Your take was saved, but history could not refresh. Reload when ready.');}
 }catch(e){if(alive.current){setError(errorMessage(e));setPhase('preview');}}}
 const locked=phase==='asking'||phase==='uploading';
 return <section className="recorder" aria-label="Microphone recording"><div className="recording-state" role="status">{phase==='recording'?`● Recording · ${Math.floor(seconds/60)}:${String(seconds%60).padStart(2,'0')}`:phase==='asking'?'Waiting for your microphone…':phase==='uploading'?'Saving your performance…':phase==='saved'?'✓ Your take is saved. Nice work!':blob?'Your take is ready to listen to.':'Ready when you are.'}</div>
  {error&&<p className="banner error" role="alert">{error}</p>}
  {blob&&phase!=='recording'&&<><audio controls src={url} aria-label="Listen to your new take"/><p className="recording-hint">{phase==='saved'?'This take is safely saved in your history.':'This take is only on this device until you save it.'}</p></>}
  <div className="recording-buttons">{phase==='recording'?<button className="stop-recording" onClick={stop}>■ Stop recording</button>:<button className="start-recording" disabled={!enabled||locked} onClick={record}>{blob?'● Record another take':'● Record'}</button>}{blob&&phase!=='recording'&&phase!=='saved'&&<button disabled={locked} onClick={save}>Save take</button>}{blob&&phase!=='recording'&&<a className="download-take" href={url} download={`storyforge-take.${blob.type.includes('mp4')?'m4a':blob.type.includes('ogg')?'ogg':'webm'}`}>Download a copy</a>}</div>
  {!enabled&&<p className="banner error">Recording storage is not ready yet. Ask Dad to finish the Unraid setup.</p>}
 </section>;
}

function ActorStudio({session,onAdmin}:{session:Session;onAdmin:()=>void}){
 const [data,setData]=useState<Workspace>(empty);const [takes,setTakes]=useState<Take[]>([]);const [sceneID,setSceneID]=useState('');const [lineID,setLineID]=useState('');const [error,setError]=useState('');const [loading,setLoading]=useState(true);const [history,setHistory]=useState(false);const dirty=useRef(false);
 const [recordingDirty,setRecordingDirty]=useState(false);
 const markDirty=useRef((value:boolean)=>{dirty.current=value;setRecordingDirty(value);}).current;
 async function loadTakes(){setTakes(await request<Take[]>('/api/actor/takes'));}
 useEffect(()=>{Promise.all([request<Workspace>('/api/actor/workspace'),request<Take[]>('/api/actor/takes')]).then(([w,t])=>{setData(w);setTakes(t);}).catch(e=>setError(errorMessage(e))).finally(()=>setLoading(false));},[]);
 useEffect(()=>{const warn=(e:BeforeUnloadEvent)=>{if(dirty.current){e.preventDefault();e.returnValue='';}};window.addEventListener('beforeunload',warn);return()=>window.removeEventListener('beforeunload',warn);},[]);
 function leave(fn:()=>void){if(!dirty.current||window.confirm('Leave this line? Your unsaved recording will be lost. Download or save it first if you want to keep it.')){dirty.current=false;fn();}}
 const assigned=new Set(data.assignments.filter(a=>a.actor_id===session.user.actor_id).map(a=>a.character_id));const mine=data.events.filter(e=>assigned.has(e.character_id));const scene=data.scenes.find(s=>s.id===sceneID);const lines=mine.filter(e=>e.scene_id===sceneID);const line=lines.find(e=>e.id===lineID)||lines[0];
 const saved=(l:Line)=>takes.some(t=>t.event_id===l.id&&t.actor_id===session.user.actor_id&&t.revision===l.revision);
 const ownTakes=takes.filter(t=>t.actor_id===session.user.actor_id);const context=data.events.filter(e=>e.scene_id===sceneID);const index=context.findIndex(e=>e.id===line?.id);const char=(id:string)=>data.characters.find(c=>c.id===id)?.name||'Character';
 return <div className="actor-studio"><header className="actor-top"><a className="actor-brand" href="#" onClick={e=>{e.preventDefault();leave(()=>{setSceneID('');setHistory(false);});}}>StoryForge <small>YOUR RECORDING STUDIO</small></a><div className="actor-account"><span>{session.user.name||session.user.email}</span>{session.user.admin&&<button className="secondary" onClick={()=>leave(onAdmin)}>Manage scripts</button>}<button className="secondary" onClick={()=>leave(()=>location.assign('/cdn-cgi/access/logout'))}>Sign out</button></div></header>
 <main className="actor-main"><div className="actor-intro"><div className="eyebrow">YOUR VOICE BRINGS THE STORY TO LIFE</div><h1>{scene?scene.name:`Your turn, ${session.user.name||'storyteller'}.`}</h1><p>{scene?'Take your time. You can try as many times as you like.':'Choose a scene, find your voice, and let’s make a story.'}</p></div>
 {error&&<p className="banner error" role="alert">{error}</p>}{loading?<p>Finding your parts…</p>:!session.user.actor_id?<div className="studio-empty"><h2>Your studio is almost ready.</h2><p>Ask Dad to link <strong>{session.user.email}</strong> to your actor under Cast & characters.</p></div>:<>
 <div className="actor-navigation"><button className="secondary" onClick={()=>leave(()=>{setSceneID('');setHistory(false);})}>My scenes</button><button className="secondary" onClick={()=>leave(()=>{setSceneID('');setHistory(true);})}>My saved takes ({ownTakes.length})</button></div>
 {history?<section><h2>Your performances</h2>{!ownTakes.length&&<p>No saved takes yet. Your first performance can start in My scenes.</p>}{ownTakes.map(t=><div key={t.id} className="history-entry"><h3>{t.project_name} · {t.scene_name} · {t.character_name}</h3><p>{t.text}</p><TakeCard take={t} onPreferred={loadTakes}/></div>)}</section>:!scene?<div className="scene-cards">{!data.scenes.length&&<div className="studio-empty"><h2>A part is on its way.</h2><p>Once your character has dialogue, your scenes will appear here.</p></div>}{data.scenes.map(s=>{const part=mine.filter(l=>l.scene_id===s.id),done=part.filter(saved).length;return <button className="scene-card" key={s.id} onClick={()=>{setSceneID(s.id);setLineID(part.find(l=>!saved(l))?.id||part[0]?.id||'');}}><span>{data.projects.find(p=>p.id===s.project_id)?.name}</span><h2>{s.name}</h2><p>{Array.from(new Set(part.map(l=>char(l.character_id)))).join(' · ')}</p><progress value={done} max={part.length||1}/><strong>{done===part.length?'All lines have a take!':`${part.length-done} lines waiting for your voice`}</strong><small>{done} of {part.length} lines recorded</small></button>})}</div>:line?<>
 <ScenePreview sceneID={scene.id} version={JSON.stringify([context,takes])} disabled={recordingDirty}/>
 <div className="line-progress"><span>{lines.filter(saved).length} of {lines.length} lines recorded</span><progress value={lines.filter(saved).length} max={lines.length}/></div>
 <div className="actor-line-picker" aria-label="Your dialogue lines">{lines.map((l,i)=><button key={l.id} className={l.id===line.id?'current':'secondary'} aria-label={`Read your line ${i+1}`} onClick={()=>leave(()=>setLineID(l.id))}>{i+1}{saved(l)?' ✓':''}</button>)}</div>
 <section className="performance-card"><div className="performance-label"><span>YOU ARE {char(line.character_id)}</span><span>Your line {lines.indexOf(line)+1} of {lines.length}</span></div>{line.direction&&<div className="performance-direction"><strong>How to say it</strong><p>{line.direction}</p></div>}<p className="performance-text">{line.text}</p><details className="surrounding-lines"><summary>What happens around this line?</summary>{context.slice(Math.max(0,index-1),index+2).filter(e=>e.id!==line.id).map(e=><p key={e.id}><strong>{char(e.character_id)}:</strong> {e.text}</p>)}</details><Recorder key={line.id+'-'+line.revision} line={line} enabled={session.recording_enabled} onSaved={loadTakes} onDirty={markDirty}/></section>
 <div className="next-line"><button className="secondary" disabled={lines.indexOf(line)===0} onClick={()=>leave(()=>setLineID(lines[lines.indexOf(line)-1].id))}>← Previous line</button><button disabled={lines.indexOf(line)===lines.length-1} onClick={()=>leave(()=>setLineID(lines[lines.indexOf(line)+1].id))}>Next line →</button></div>
 <section className="take-history"><h2>Your takes for this line</h2><p>Every saved take stays here. A star marks your preferred performance.</p>{ownTakes.filter(t=>t.event_id===line.id).map(t=><TakeCard key={t.id} take={t} onPreferred={loadTakes}/>)}{!ownTakes.some(t=>t.event_id===line.id)&&<p className="studio-empty">Your first saved take will appear here.</p>}</section>
 </>:<p>No assigned lines remain in this scene.</p>}</>}
 <footer className="actor-footer">A little imagination. A voice all your own.</footer></main></div>;
}

function Review({onBack}:{onBack:()=>void}){
 const [takes,setTakes]=useState<Take[]>([]);const [data,setData]=useState<Workspace>(empty);const [error,setError]=useState('');const [project,setProject]=useState('');
 async function load(){const [t,w]=await Promise.all([request<Take[]>('/api/actor/takes'),request<Workspace>('/api/workspace')]);setTakes(t);setData(w);}
 useEffect(()=>{load().catch(e=>setError(errorMessage(e)));},[]);
 const events=data.events.filter(e=>!project||e.project_id===project);const ids=new Set(events.map(e=>e.id));const recorded=events.filter(e=>takes.some(t=>t.event_id===e.id&&t.revision===e.revision)).length;
 return <div className="review-studio"><button className="secondary" onClick={onBack}>← Manage scripts</button><h1>Listen to the story taking shape.</h1><p>{recorded} of {events.length} lines have current recordings.</p><label>Project<select value={project} onChange={e=>setProject(e.target.value)}><option value="">All projects</option>{data.projects.map(p=><option key={p.id} value={p.id}>{p.name}</option>)}</select></label>{error&&<p role="alert">{error}</p>}{takes.filter(t=>ids.has(t.event_id)).map(t=><div className="history-entry" key={t.id}><h2>{t.actor_name} as {t.character_name}</h2><p>{t.project_name} · {t.scene_name}</p><p>{t.text}</p><TakeCard take={t} onPreferred={load}/></div>)}</div>;
}
