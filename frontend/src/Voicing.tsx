import {useEffect,useRef,useState,type FormEvent} from 'react';
import {request} from './api';

type Voice={voice_id:string;name:string};
type Library={configured:boolean;voices:Voice[]};
type Save=(path:string,body:unknown,method?:string)=>Promise<boolean>;
export function TargetVoice({id,name,value,label,busy,save}:{id:string;name:string;value:string;label:string;busy:boolean;save:Save}){
 const [voice,setVoice]=useState(value);const [library,setLibrary]=useState<Library>();const [error,setError]=useState('');const [loading,setLoading]=useState(false);
 useEffect(()=>setVoice(value),[value]);
 async function load(refresh=false){setLoading(true);setError('');try{setLibrary(await request<Library>(refresh?'/api/voices/refresh':'/api/voices',refresh?'POST':'GET',refresh?{}:undefined));}catch(e){setError(message(e));}finally{setLoading(false);}}
 useEffect(()=>{void load();},[]);
 async function submit(e:FormEvent){e.preventDefault();await save(`/api/characters/${id}/target-voice`,{voice_id:voice},'PUT');}
 const unavailable=value&&!library?.voices.some(v=>v.voice_id===value);
 return <form className="target-voice" onSubmit={submit}><label>Target voice<select aria-label={`Target voice for ${name}`} value={voice} onChange={e=>setVoice(e.target.value)} disabled={busy||loading||!library?.configured}><option value="">Choose a voice</option>{unavailable&&<option value={value} disabled>{label||value} — unavailable; refresh voices</option>}{library?.voices.map(v=><option key={v.voice_id} value={v.voice_id}>{v.name}</option>)}</select></label><button className="secondary" disabled={busy||loading||voice===value} aria-label={`Save target voice for ${name}`}>Save</button><button type="button" className="secondary" disabled={loading||busy} onClick={()=>void load(true)} aria-label={`Refresh voices for ${name}`}>{loading?'Loading voices…':'Refresh voices'}</button><small>{library&&!library.configured?'Add ELEVENLABS_API_KEY in Unraid to connect your voice library.':!value&&label?`Previously entered: ${label}. Choose its ElevenLabs voice above.`:'Create voices in ElevenLabs, then click Refresh voices here. Lists are cached for 10 minutes.'}</small>{error&&<small role="alert">{error}</small>}</form>;
}
const message=(e:unknown)=>e instanceof Error?e.message:'Unable to load voice conversion.';
type Line={event_id:string;take_id:string;voice_id:string;character:string;text:string;position:number};
type Conversion={id:string;event_id:string;take_id:string;voice_id:string};
type State={configured:boolean;ready:boolean;missing_takes:number;missing_voices:number;snapshot_hash:string;lines:Line[];run:null|{id:string;state:string;error:string;done:number;total:number};finished:null|{id:string;snapshot_hash:string};conversions:Conversion[]};
function ConvertedPlayer({src,label}:{src:string;label:string}){return <audio controls preload="none" src={src} aria-label={label} onPlay={e=>{document.querySelectorAll('audio').forEach(a=>{if(a!==e.currentTarget)a.pause();});}}/>;}
export function Voicing({sceneID,admin=false,eventID,version,disabled=false}:{sceneID:string;admin?:boolean;eventID?:string;version:string;disabled?:boolean}){
 const [state,setState]=useState<State>();const [error,setError]=useState('');const [busy,setBusy]=useState(false);const retry=useRef<{key:string;id:string}|undefined>(undefined);
 const current=useRef(0);
 useEffect(()=>{
  const generation=++current.current;let stopped=false;let timer:ReturnType<typeof setTimeout>;
  setState(undefined);setError('');setBusy(false);retry.current=undefined;
  async function poll(){try{const s=await request<State>(`/api/actor/scenes/${sceneID}/voicing`);if(!stopped){setState(s);}}catch(e){if(!stopped)setError(message(e));}finally{if(!stopped)timer=setTimeout(poll,3000);}}
  void poll();return()=>{stopped=true;clearTimeout(timer);if(current.current===generation)current.current++;};
 },[sceneID,version]);
 async function start(event=''){
  if(!state)return;
  if(!window.confirm(event?'Regenerate this line using ElevenLabs credits, then rebuild the scene?':'Send the selected recordings to ElevenLabs? New conversions use your ElevenLabs credits. Completed matching lines will be reused.'))return;
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
  return <div className="converted-line" key={line.event_id}><strong>{line.position}. {line.character}</strong><p>{line.text}</p>{c?<><small>{stale?'Earlier take or voice — needs updating':'Latest converted line'}</small>{!disabled&&<ConvertedPlayer src={audio(c.id)} label={`Converted line ${line.position}`}/>}</>:<p>No converted recording yet.</p>}{admin&&c&&<button className="secondary" disabled={busy||!!active||disabled||!state?.ready||!state.configured} onClick={()=>void start(line.event_id)}>Regenerate line {line.position}</button>}</div>;
 };
 return <section className="voicing" aria-label="ElevenLabs scene"><h3>Character voices</h3>
 {error&&<p role="alert" className="banner error">{error}</p>}
 {!state?<p>Loading conversion status…</p>:<>
 {admin&&<><p>{!state.configured?'Connect ElevenLabs in Unraid to voice this scene.':state.ready?'Ready to convert the selected takes.':`${state.missing_takes} lines need current takes · ${state.missing_voices} lines need target voices`}</p><button disabled={busy||!!active||!state.ready||!state.configured||disabled} onClick={()=>void start()}>{busy?'Queuing…':active?'Voicing scene…':'Voice scene'}</button></>}
 {state.run&&<p className="voice-progress">{state.run.state==='complete'?'Scene conversion complete':state.run.state==='failed'?'Conversion needs attention':`${state.run.state==='queued'?'Queued':'Converting'}: ${state.run.done} of ${state.run.total} lines ready`}</p>}
 {state.run?.error&&<p className="banner error">{state.run.error}</p>}
 {state.finished&&<div className="converted-scene"><strong>Converted scene</strong>{state.finished.snapshot_hash!==state.snapshot_hash&&<p>Earlier scene version — new takes, voices, or script changes need conversion.</p>}{!disabled&&<ConvertedPlayer src={audio(state.finished.id)} label="Play converted scene"/>}</div>}
 {eventID?state.lines.filter(l=>l.event_id===eventID).map(renderLine):<details><summary>Converted lines</summary>{state.lines.map(renderLine)}</details>}
 </>}</section>;
}
