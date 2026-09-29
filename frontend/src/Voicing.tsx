import {useEffect,useRef,useState,type FormEvent} from 'react';
import {request} from './api';
import type {Take} from './Studio';

type Voice={voice_id:string;name:string};
type Library={configured:boolean;voices:Voice[]};
type Save=(path:string,body:unknown,method?:string)=>Promise<boolean>;
export function RefreshVoices(){
 const [busy,setBusy]=useState(false);const [notice,setNotice]=useState('');
 async function refresh(){setBusy(true);setNotice('');try{await request('/api/voices/refresh','POST',{});window.dispatchEvent(new Event('storyforge-voices-refreshed'));setNotice('Voice list refreshed.');}catch(e){setNotice(message(e));}finally{setBusy(false);}}
 return <div className="refresh-voices"><button className="secondary" disabled={busy} onClick={()=>void refresh()}>{busy?'Refreshing voices…':'Refresh Voices'}</button>{notice&&<small role="status">{notice}</small>}</div>;
}
export function TargetVoice({id,name,value,label,busy,save}:{id:string;name:string;value:string;label:string;busy:boolean;save:Save}){
 const [voice,setVoice]=useState(value);const [library,setLibrary]=useState<Library>();const [error,setError]=useState('');const [loading,setLoading]=useState(false);
 useEffect(()=>setVoice(value),[value]);
 async function load(refresh=false){setLoading(true);setError('');try{setLibrary(await request<Library>(refresh?'/api/voices/refresh':'/api/voices',refresh?'POST':'GET',refresh?{}:undefined));}catch(e){setError(message(e));}finally{setLoading(false);}}
 useEffect(()=>{void load();const refreshed=()=>void load();window.addEventListener('storyforge-voices-refreshed',refreshed);return()=>window.removeEventListener('storyforge-voices-refreshed',refreshed);},[]);
 async function submit(e:FormEvent){e.preventDefault();await save(`/api/characters/${id}/target-voice`,{voice_id:voice},'PUT');}
 const unavailable=value&&!library?.voices.some(v=>v.voice_id===value);
 return <form className="target-voice" onSubmit={submit}><label>Target voice<select aria-label={`Target voice for ${name}`} value={voice} onChange={e=>setVoice(e.target.value)} disabled={busy||loading||!library?.configured}><option value="">Choose a voice</option>{unavailable&&<option value={value} disabled>{label||value} — unavailable; refresh voices</option>}{library?.voices.map(v=><option key={v.voice_id} value={v.voice_id}>{v.name}</option>)}</select></label><button className="secondary" disabled={busy||loading||voice===value} aria-label={`Save target voice for ${name}`}>Save</button><small>{library&&!library.configured?'Add ELEVENLABS_API_KEY in Unraid to connect your voice library.':!value&&label?`Previously entered: ${label}. Choose its ElevenLabs voice above.`:'Choose Isolation to clean up the original voice, or choose a character voice. After creating voices in ElevenLabs, use Refresh Voices in the top bar.'}</small>{error&&<small role="alert">{error}</small>}</form>;
}
const message=(e:unknown)=>e instanceof Error?e.message:'Unable to load voice conversion.';
type Line={event_id:string;take_id:string;voice_id:string;character:string;text:string;position:number};
type Conversion={id:string;event_id:string;take_id:string;voice_id:string};
type State={configured:boolean;ready:boolean;missing_takes:number;missing_voices:number;snapshot_hash:string;lines:Line[];run:null|{id:string;state:string;single_line:boolean;error:string;done:number;total:number};finished:null|{id:string;snapshot_hash:string};conversions:Conversion[]};
function ConvertedPlayer({src,label}:{src:string;label:string}){return <audio controls preload="none" src={src} aria-label={label} onPlay={e=>{document.querySelectorAll('audio').forEach(a=>{if(a!==e.currentTarget)a.pause();});}}/>;}
type AuditionTake=Take&{eligible:boolean};
function TakeAudition({line,takes,disabled,onPrefer}:{line:Line;takes:AuditionTake[];disabled:boolean;onPrefer:(id:string)=>Promise<void>}){
 const [selected,setSelected]=useState('');
 const take=takes.find(t=>t.id===selected)||takes.find(t=>t.id===line.take_id)||takes[0];
 if(!take)return <p>No recorded takes for this line yet.</p>;
 return <div className="take-audition"><label>Recorded takes<select aria-label={`Take for line ${line.position}`} value={take.id} disabled={disabled} onChange={e=>setSelected(e.target.value)}>{takes.map(t=><option key={t.id} value={t.id}>{t.actor_name} · Take {t.take_number} · {new Date(t.created_at).toLocaleString()}{t.id===line.take_id?' · Preferred for conversion':''}{t.stale?' · Earlier script version':''}</option>)}</select></label>
 {!disabled&&<ConvertedPlayer key={take.id} src={`/api/actor/takes/${take.id}/audio`} label={`Play selected take for line ${line.position}`}/>}
 <button className="secondary" aria-label={`Make selected take preferred for line ${line.position}`} disabled={disabled||!take.eligible||take.id===line.take_id} onClick={()=>void onPrefer(take.id)}>{take.id===line.take_id?'Preferred for conversion':'Make preferred'}</button>
 <small>Choosing a take here only changes playback. Make it preferred to use it for conversion.</small>
 {take.stale&&<details><summary>Words recorded in this take</summary><p>{take.text}</p>{take.direction&&<p>{take.direction}</p>}</details>}
 </div>;
}
export function Voicing({sceneID,admin=false,eventID,version,disabled=false,review=false}:{sceneID:string;admin?:boolean;eventID?:string;version:string;disabled?:boolean;review?:boolean}){
 const [state,setState]=useState<State>();const [error,setError]=useState('');const [busy,setBusy]=useState(false);const retry=useRef<{key:string;id:string}|undefined>(undefined);
 const [takes,setTakes]=useState<AuditionTake[]>([]);
 const current=useRef(0);const selectionVersion=useRef(0);
 useEffect(()=>{
  const generation=++current.current;let stopped=false;let timer:ReturnType<typeof setTimeout>;
  setState(undefined);setTakes([]);setError('');setBusy(false);retry.current=undefined;
  async function poll(){const selection=selectionVersion.current;try{const [s,t]=await Promise.all([request<State>(`/api/actor/scenes/${sceneID}/voicing`),admin?request<AuditionTake[]>(`/api/actor/takes?scene_id=${sceneID}`):Promise.resolve([])]);if(!stopped&&selection===selectionVersion.current){setState(s);setTakes(t);}}catch(e){if(!stopped)setError(message(e));}finally{if(!stopped)timer=setTimeout(poll,3000);}}
  void poll();return()=>{stopped=true;clearTimeout(timer);if(current.current===generation)current.current++;};
 },[sceneID,version,admin]);
 async function prefer(id:string){
  const generation=current.current;selectionVersion.current++;setBusy(true);setError('');
  try{await request(`/api/actor/takes/${id}/preferred`,'PUT',{preferred:true});const [s,t]=await Promise.all([request<State>(`/api/actor/scenes/${sceneID}/voicing`),request<AuditionTake[]>(`/api/actor/takes?scene_id=${sceneID}`)]);if(current.current===generation){selectionVersion.current++;setState(s);setTakes(t);retry.current=undefined;}}
  catch(e){if(current.current===generation)setError(message(e));}finally{if(current.current===generation)setBusy(false);}
 }
 async function start(event=''){
  if(!state)return;
  if(!window.confirm(event?'Convert this line using ElevenLabs credits? Generate a scene preview afterward to hear it in context.':'Send the selected recordings to ElevenLabs? Voice changing and isolation both use your ElevenLabs credits. Completed matching lines will be reused.'))return;
  const generation=current.current;const key=state.snapshot_hash+event;
  if(retry.current?.key!==key)retry.current={key,id:crypto.randomUUID()};
  setBusy(true);setError('');
  try{await request(`/api/scenes/${sceneID}/voice`,'POST',{request_id:retry.current.id,snapshot_hash:state.snapshot_hash,event_id:event});if(current.current!==generation)return;retry.current=undefined;setState(await request<State>(`/api/actor/scenes/${sceneID}/voicing`));}
  catch(e){if(current.current===generation)setError(message(e));}finally{if(current.current===generation)setBusy(false);}
 }
 const active=state?.run&&['queued','running'].includes(state.run.state);
 const audio=(id:string)=>`/api/actor/scenes/${sceneID}/converted/${id}`;
 const renderLine=(line:Line)=>{
  const c=state?.conversions.find(c=>c.event_id===line.event_id);const stale=c&&(c.take_id!==line.take_id||c.voice_id!==line.voice_id);
  return <div className="converted-line" key={line.event_id}><strong>{line.position}. {line.character}</strong><p>{line.text}</p>{admin&&<TakeAudition line={line} takes={takes.filter(t=>t.event_id===line.event_id)} disabled={disabled||busy||!!active} onPrefer={prefer}/>} {c?<><small>{stale?'Changed since processing — needs updating':'Up to date · Latest converted line'}</small>{!disabled&&<ConvertedPlayer src={audio(c.id)} label={`Converted line ${line.position}`}/>}</>:<p>No converted recording yet.</p>}{admin&&<button className="secondary" disabled={busy||!!active||disabled||!line.take_id||!line.voice_id||!state?.configured||(review&&!!c&&!stale)} onClick={()=>void start(line.event_id)}>{review?'Process':c?'Regenerate':'Convert'} line {line.position}</button>}</div>;
 };
 return <section className="voicing" aria-label="ElevenLabs scene"><h3>Character voices</h3>
 {error&&<p role="alert" className="banner error">{error}</p>}
 {!state?<p>Loading conversion status…</p>:<>
 {admin&&<><p>{!state.configured?'Connect ElevenLabs in Unraid to voice this scene.':state.ready?'Ready to convert the selected takes.':`${state.missing_takes} lines need current takes · ${state.missing_voices} lines need target voices`}</p><button disabled={busy||!!active||!state.ready||!state.configured||disabled} onClick={()=>void start()}>{busy?'Queuing…':active?'Voicing scene…':'Voice scene'}</button></>}
 {state.run&&<p className="voice-progress">{state.run.state==='complete'?(state.run.single_line?'Line conversion complete — generate a scene preview to hear it in context.':'Scene conversion complete'):state.run.state==='failed'?'Conversion needs attention':`${state.run.state==='queued'?'Queued':'Converting'}: ${state.run.done} of ${state.run.total} lines ready`}</p>}
 {state.run?.error&&<p className="banner error">{state.run.error}</p>}
 {state.finished&&<div className="converted-scene"><strong>Converted scene</strong>{(state.finished.snapshot_hash!==state.snapshot_hash||state.run?.single_line)&&<p>Earlier scene version — new takes, voices, or script changes need conversion.</p>}{!disabled&&<ConvertedPlayer src={audio(state.finished.id)} label="Play converted scene"/>}</div>}
 {eventID?state.lines.filter(l=>l.event_id===eventID).map(renderLine):<details open={review||undefined}><summary>{review?'Recordings and processing':'Converted lines'}</summary>{state.lines.map(renderLine)}</details>}
 </>}</section>;
}
