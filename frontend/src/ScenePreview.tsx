import {useEffect,useRef,useState} from 'react';
import {request} from './api';

type Preview={url:string;recorded_lines:number;synthetic_lines:number};
export function ScenePreview({sceneID,version,disabled=false}:{sceneID:string;version:string;disabled?:boolean}) {
 const [preview,setPreview]=useState<Preview>();
 const [busy,setBusy]=useState(false);
 const [error,setError]=useState('');
 const generation=useRef(0);
 useEffect(()=>{generation.current++;setPreview(undefined);setError('');setBusy(false);return()=>{generation.current++;};},[sceneID,version]);
 async function generate(){
  const current=++generation.current;setBusy(true);setError('');setPreview(undefined);
  document.querySelectorAll('audio').forEach(a=>a.pause());
  try {const result=await request<Preview>(`/api/actor/scenes/${sceneID}/preview`,'POST',{});if(generation.current===current)setPreview(result);}
  catch(e){if(generation.current===current)setError(e instanceof Error?e.message:'Unable to generate preview.');}
  finally{if(generation.current===current)setBusy(false);}
 }
 return <section className="scene-preview" aria-label="Whole scene preview">
  <h3>Hear the whole scene</h3>
  <p>All roles, in script order. Saved performances replace the basic computer voices. Generate again to include the latest takes.</p>
  <button className="secondary" disabled={busy||disabled} onClick={generate}>{busy?'Building your scene…':preview?'Update scene preview':'Generate scene preview'}</button>
  {busy&&<p role="status">The server is putting all the voices together. This may take a moment.</p>}
  {preview&&<><p role="status">{preview.recorded_lines} recorded · {preview.synthetic_lines} computer-voiced lines</p>{!disabled&&<audio controls preload="metadata" src={preview.url} aria-label="Play whole scene" onPlay={e=>{const current=e.currentTarget;document.querySelectorAll('audio').forEach(a=>{if(a!==current)a.pause();});}}/>}</>}
  {error&&<p role="alert" className="banner error">{error}</p>}
 </section>;
}
