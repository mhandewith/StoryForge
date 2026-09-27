import {useEffect,useRef,useState} from 'react';
import {request} from './api';

type Preview={url:string;recorded_lines:number;converted_lines:number;synthetic_lines:number;part:number;parts:number;start_line:number;end_line:number;snapshot:string};
export function ScenePreview({sceneID,version,disabled=false}:{sceneID:string;version:string;disabled?:boolean}) {
 const [previews,setPreviews]=useState<Preview[]>([]);
 const [busy,setBusy]=useState(false);
 const [error,setError]=useState('');
 const generation=useRef(0);
 useEffect(()=>{generation.current++;setPreviews([]);setError('');setBusy(false);return()=>{generation.current++;};},[sceneID,version]);
 async function generate(){
  const current=++generation.current;setBusy(true);setError('');setPreviews([]);
  document.querySelectorAll('audio').forEach(a=>a.pause());
  try {
   let snapshot='',parts=1;
   for(let part=0;part<parts;part++) {
    if(generation.current!==current)return;
    const result=await request<Preview>(`/api/actor/scenes/${sceneID}/preview`,'POST',{part,snapshot});
    if(generation.current!==current)return;
    snapshot=result.snapshot;parts=result.parts;
    setPreviews(previous=>[...previous,result]);
   }
  }
  catch(e){if(generation.current===current)setError(e instanceof Error?e.message:'Unable to generate preview.');}
  finally{if(generation.current===current)setBusy(false);}
 }
 return <section className="scene-preview" aria-label="Whole scene preview">
  <h3>Hear the whole scene</h3>
  <p>All roles, in script order. Matching converted lines play first, followed by raw takes, then basic computer voices for unrecorded lines. Long scenes are automatically divided into listening parts. Generate again to include the latest takes.</p>
  <button className="secondary" disabled={busy||disabled} onClick={generate}>{busy?'Building your scene…':previews.length?'Update scene preview':'Generate scene preview'}</button>
  {busy&&<p role="status">{previews.length?`Building part ${Math.min(previews.length+1,previews[0].parts)} of ${previews[0].parts}… You can listen to finished parts below.`:'The server is putting the voices together. This may take a moment.'}</p>}
  {previews.map(preview=><div key={preview.part}>
   {preview.parts>1&&<h4>Part {preview.part+1} of {preview.parts} · Lines {preview.start_line}–{preview.end_line}</h4>}
   <p role="status">{preview.converted_lines>0?`${preview.converted_lines} converted · ${preview.recorded_lines-preview.converted_lines} raw takes`: `${preview.recorded_lines} recorded`} · {preview.synthetic_lines} computer-voiced lines</p>
   {!disabled&&<audio controls preload="metadata" src={preview.url} aria-label={preview.parts>1?`Play scene part ${preview.part+1}`:'Play whole scene'} onPlay={e=>{const current=e.currentTarget;document.querySelectorAll('audio').forEach(a=>{if(a!==current)a.pause();});}}/>}
  </div>)}
  {error&&<p role="alert" className="banner error">{error}</p>}
 </section>;
}
