import { useEffect, useState, type FormEvent, type ReactNode } from 'react';
import { empty, request, type Workspace, type Named, type Line } from './api';

type Save = (path:string,body:unknown,method?:string)=>Promise<boolean>;
function Card({title,description,children}:{title:string;description:string;children:ReactNode}) {
  return <section className="card"><div className="section-heading"><h2>{title}</h2><p>{description}</p></div>{children}</section>;
}
function NameForm({label,button,onSave,disabled=false}:{label:string;button:string;onSave:(name:string)=>Promise<boolean>;disabled?:boolean}) {
  const [name,setName]=useState('');
  async function submit(e:FormEvent) { e.preventDefault(); if(await onSave(name))setName(''); }
  return <form className="inline-form" onSubmit={submit}><label>{label}<input value={name} onChange={e=>setName(e.target.value)} required maxLength={120} disabled={disabled}/></label><button disabled={disabled}>{button}</button></form>;
}
function LineForm({line,projectID,sceneID,characters,nextPosition,save,busy,cancel}:{line?:Line;projectID:string;sceneID:string;characters:Named[];nextPosition:number;save:Save;busy:boolean;cancel:()=>void}) {
  const [character,setCharacter]=useState(line?.character_id||characters[0]?.id||'');
  const [text,setText]=useState(line?.text||'');
  const [direction,setDirection]=useState(line?.direction||'');
  const [position,setPosition]=useState(line?.position||nextPosition);
  const [start,setStart]=useState(line?.start_ms||0);
  async function submit(e:FormEvent) {
    e.preventDefault();
    const body={project_id:projectID,scene_id:sceneID,character_id:character,text,direction,position,start_ms:start,...(line?{revision:line.revision}:{})};
    if(await save(line?`/api/events/${line.id}`:'/api/events',body,line?'PUT':'POST')) {
      if(line)cancel(); else {setText('');setDirection('');setPosition(position+1);}
    }
  }
  return <form className="line-form" onSubmit={submit}>
    <h3>{line?'Edit dialogue':'Add dialogue'}</h3>
    <fieldset disabled={busy||characters.length===0}>
      <div className="form-grid"><label>Character<select required value={character} onChange={e=>setCharacter(e.target.value)}><option value="" disabled>Choose a character</option>{characters.map(c=><option key={c.id} value={c.id}>{c.name}</option>)}</select></label>
      <label>Line position<input type="number" required min={1} max={2147483647} step={1} value={position} onChange={e=>setPosition(e.target.valueAsNumber)}/></label>
      <label>Start time (ms)<input type="number" required min={0} max={86400000} step={1} value={start} onChange={e=>setStart(e.target.valueAsNumber)}/></label></div>
      <label>Dialogue<textarea aria-label="Dialogue" required maxLength={10000} rows={3} value={text} onChange={e=>setText(e.target.value)} placeholder="What does the character say?"/></label>
      <label>Performance direction <span className="optional">optional</span><input maxLength={2000} value={direction} onChange={e=>setDirection(e.target.value)} placeholder="For example: quietly, with a little excitement"/></label>
      <p className="hint">Position controls reading order. Start time places the line on the future scene timeline; lines can share a start time.</p>
      <div className="actions"><button>{busy?'Saving…':line?'Save changes':'Add line'}</button>{line&&<button className="secondary" type="button" onClick={cancel}>Cancel</button>}</div>
    </fieldset>
  </form>;
}

export default function App() {
  const [data,setData]=useState<Workspace>(empty);
  const [projectID,setProjectID]=useState('');
  const [sceneID,setSceneID]=useState('');
  const [editing,setEditing]=useState<Line>();
  const [loading,setLoading]=useState(true);
  const [busy,setBusy]=useState(false);
  const [error,setError]=useState('');
  const [notice,setNotice]=useState('');
  const [tab,setTab]=useState<'script'|'cast'>('script');
  async function refresh() {
    const workspace=await request<Workspace>('/api/workspace'); setData(workspace);
    setProjectID(id=>workspace.projects.some(p=>p.id===id)?id:workspace.projects[0]?.id||'');
    return workspace;
  }
  useEffect(()=>{refresh().catch(e=>setError(e.message||'Cannot reach StoryForge.')).finally(()=>setLoading(false));},[]);
  const project=data.projects.find(p=>p.id===projectID);
  const scenes=data.scenes.filter(s=>s.project_id===projectID);
  const characters=data.characters.filter(c=>c.project_id===projectID);
  const scene=scenes.find(s=>s.id===sceneID)||scenes[0];
  const lines=data.events.filter(e=>e.scene_id===scene?.id);
  const nextPosition=Math.max(0,...lines.map(l=>l.position))+1;
  async function save(path:string,body:unknown,method='POST') {
    setBusy(true);setError('');setNotice('');
    try {
      const item=await request<Named>(path,method,body);
      try {await refresh();} catch {setError('Saved, but the list could not refresh. Use Reload before making more changes.');return true;}
      if(path==='/api/projects'){setProjectID(item.id);setSceneID('');setEditing(undefined);}
      if(path==='/api/scenes'){setSceneID(item.id);setEditing(undefined);}
      setNotice('Saved to StoryForge.');return true;
    } catch(e) {setError(e instanceof Error?e.message:'Unable to save.');return false;}
    finally{setBusy(false);}
  }
  function changeProject(id:string){setProjectID(id);setSceneID('');setEditing(undefined);setNotice('');}
  async function reload(){setBusy(true);setError('');try{await refresh();setEditing(undefined);setNotice('Reloaded from the server.');}catch(e){setError(e instanceof Error?e.message:'Unable to reload.');}finally{setBusy(false);}}
  const actorFor=(characterID:string)=>data.actors.find(a=>a.id===data.assignments.find(x=>x.character_id===characterID)?.actor_id)?.name;
  return <div className="app">
    <aside className="sidebar"><a className="brand" href="/"><span className="brand-icon">S</span><span>StoryForge<small>THE SCRIPT STUDIO</small></span></a>
      <div className="sidebar-label">YOUR STORIES</div>
      <label className="project-picker">Current project<select disabled={busy||loading} value={projectID} onChange={e=>changeProject(e.target.value)}><option value="" disabled>Choose a project</option>{data.projects.map(p=><option key={p.id} value={p.id}>{p.name}</option>)}</select></label>
      <nav aria-label="Studio"><button className={tab==='script'?'nav-button selected':'nav-button'} onClick={()=>setTab('script')}>▤ <span>Scenes & script</span></button><button className={tab==='cast'?'nav-button selected':'nav-button'} onClick={()=>setTab('cast')}>♧ <span>Cast & characters</span></button></nav>
      <div className="sidebar-note"><span className="small-star">✦</span><p>A little imagination.<br/>A story all your own.</p></div><div className="sidebar-footer">ADMIN WORKSPACE <span>02</span></div>
    </aside>
    <main><header className="topbar"><span>WORKSPACE <span className="crumb">/</span> {tab==='script'?'Script studio':'Your cast'}</span><button className="secondary" onClick={reload} disabled={busy||loading}>Reload</button></header>
      <div className="content"><div className="hero"><div><div className="eyebrow">LET’S MAKE A STORY</div><h1>{project?.name||'Every story starts here.'}</h1><p>{tab==='script'?'Set the scene. Find the words. Bring everyone into the story.':'Meet the people and characters who make your story their own.'}</p></div><span className="hero-mark" aria-hidden="true">✦</span></div>
        {error&&<div className="banner error" role="alert">{error}</div>}{notice&&<div className="banner notice" role="status">{notice}</div>}
        {loading?<p className="empty">Opening your studio…</p>:<>
          <div className="stats"><div><strong>{scenes.length}</strong><span>SCENES</span></div><div><strong>{characters.length}</strong><span>CHARACTERS</span></div><div><strong>{data.events.filter(e=>e.project_id===projectID).length}</strong><span>DIALOGUE LINES</span></div><p>Your script is the start.<br/><span>Recording comes next.</span></p></div>
          <details className="project-create" open={!project}><summary>Create a new project</summary><NameForm label="Project name" button="Create project" disabled={busy} onSave={name=>save('/api/projects',{name})}/></details>
          {!project?<div className="welcome"><span>01 / BEGIN</span><h2>Make room for your first story.</h2><p>Create a project above, then add your cast and a scene.</p></div>:tab==='cast'?<div className="cast-layout">
            <Card title="The people behind the voices" description="Actors are available across all your projects."><NameForm label="Actor name" button="Add actor" disabled={busy} onSave={name=>save('/api/actors',{name})}/><div className="actor-list">{data.actors.map(a=><div key={a.id}><span className="avatar">{a.name.slice(0,1)}</span>{a.name}</div>)}</div>{!data.actors.length&&<p className="empty">Add Hazel, Hannah, or anyone joining the story.</p>}</Card>
            <Card title="Characters in this story" description="Give each role a name and choose who will play it."><NameForm label="Character name" button="Add character" disabled={busy} onSave={name=>save('/api/characters',{name,project_id:projectID})}/><div className="character-list">{characters.map(c=><div className="character-row" key={c.id}><strong>{c.name}</strong><label>Played by<select aria-label={`Actor for ${c.name}`} disabled={busy||!data.actors.length} value={data.assignments.find(a=>a.character_id===c.id)?.actor_id||''} onChange={e=>void save(`/api/assignments/${c.id}`,{actor_id:e.target.value},'PUT')}><option value="" disabled>Choose an actor</option>{data.actors.map(a=><option key={a.id} value={a.id}>{a.name}</option>)}</select></label></div>)}</div>{!characters.length&&<p className="empty">Your characters will appear here.</p>}</Card>
          </div>:<div className="script-layout"><section className="scene-panel"><div className="section-heading"><h2>Scenes <span className="count">{scenes.length}</span></h2><p>One chapter of the adventure at a time.</p></div><div className="scene-list">{scenes.map(s=><button disabled={busy} className={scene?.id===s.id?'scene selected':'scene'} key={s.id} onClick={()=>{setSceneID(s.id);setEditing(undefined);}}><span className="scene-number">{String(s.position).padStart(2,'0')}</span><span>{s.name}<small>{data.events.filter(l=>l.scene_id===s.id).length} lines</small></span></button>)}</div><NameForm label="Scene name" button="Add scene" disabled={busy} onSave={name=>save('/api/scenes',{name,project_id:projectID,position:Math.max(0,...scenes.map(s=>s.position))+1})}/></section>
            <section className="script-panel">{scene?<><div className="script-heading"><div className="eyebrow">SCENE {String(scene.position).padStart(2,'0')}</div><h2>{scene.name}</h2><p>{lines.length} dialogue {lines.length===1?'line':'lines'} <span>•</span> Draft your next adventure</p></div>
              {!characters.length?<div className="empty"><p>Add a character before writing dialogue.</p><button className="secondary" onClick={()=>setTab('cast')}>Set up your cast</button></div>:<>
                <div className="lines">{lines.map(l=><article className="dialogue" key={l.id}><span className="line-number">{String(l.position).padStart(2,'0')}</span><div><div className="line-meta"><strong>{characters.find(c=>c.id===l.character_id)?.name}</strong><span>{actorFor(l.character_id)||'Unassigned'} · {l.start_ms} ms</span></div><p className="dialogue-text">{l.text}</p>{l.direction&&<p className="direction">{l.direction}</p>}</div><button className="text-button" disabled={busy} aria-label={`Edit line ${l.position}`} onClick={()=>setEditing(l)}>Edit</button></article>)}</div>
                {!lines.length&&<p className="empty">A blank page, a world of possibilities. Write your first line below.</p>}
                <LineForm key={editing?.id||`${scene.id}-${characters.map(c=>c.id).join()}`} line={editing} projectID={projectID} sceneID={scene.id} characters={characters} nextPosition={nextPosition} save={save} busy={busy} cancel={()=>setEditing(undefined)}/>
              </>}</>:<div className="welcome"><span>02 / SET THE SCENE</span><h2>Where does your story begin?</h2><p>Add a scene to start writing dialogue.</p></div>}</section></div>}
        </>}
        <footer className="page-footer">Made for stories worth telling.<span>STORYFORGE / SCRIPT STUDIO</span></footer>
      </div>
    </main>
  </div>;
}
