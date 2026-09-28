import {useState} from 'react';
export function ExportAudio({projectID}:{projectID:string}){
 const [busy,setBusy]=useState(false);const [error,setError]=useState('');
 async function download(){setBusy(true);setError('');try{
  const response=await fetch(`/api/projects/${projectID}/export`);
  if(!response.ok||!response.headers.get('content-type')?.includes('application/zip')){const data=await response.json().catch(()=>({error:'Your login may have expired. Reload and try again.'}));throw new Error(data.error||'Export failed.');}
  const url=URL.createObjectURL(await response.blob());const a=document.createElement('a');a.href=url;a.download=response.headers.get('content-disposition')?.match(/filename="([^"]+)"/)?.[1]||'storyforge-audio.zip';a.click();setTimeout(()=>URL.revokeObjectURL(url),60000);
 }catch(e){setError(e instanceof Error?e.message:'Unable to export.');}finally{setBusy(false);}}
 return <div className="export-audio"><button className="secondary" disabled={!projectID||busy} onClick={()=>void download()}>{busy?'Preparing audio…':'Export project audio'}</button>{error&&<p role="alert" className="banner error">{error}</p>}</div>;
}
