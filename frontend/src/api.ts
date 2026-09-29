export type Named = {id:string;name:string};
export type Scene = Named & {project_id:string;position:number};
export type Character = Named & {project_id:string;target_voice:string;eleven_voice_id:string};
export type Line = {id:string;project_id:string;scene_id:string;character_id:string;text:string;direction:string;position:number;start_ms:number;revision:number};
export type DialogueGroup={id:string;scene_id:string;members:{event_id:string;offset_ms:number}[]};
export type DialogueTransition={scene_id:string;predecessor:string;successor:string;offset_ms:number};
export type Workspace = {projects:Named[];actors:Named[];scenes:Scene[];characters:Character[];assignments:{character_id:string;actor_id:string|null}[];events:Line[];dialogue_groups:DialogueGroup[];dialogue_transitions:DialogueTransition[]};
export const empty:Workspace={projects:[],actors:[],scenes:[],characters:[],assignments:[],events:[],dialogue_groups:[],dialogue_transitions:[]};
export async function request<T>(path:string,method='GET',body?:unknown):Promise<T> {
  const response=await fetch(path,{method,headers:body===undefined?{}:{'Content-Type':'application/json'},body:body===undefined?undefined:JSON.stringify(body)});
  if(!response.headers.get('content-type')?.includes('application/json'))throw new Error('Your login may have expired. Save or download any local recording before signing in again.');
  const data=await response.json();
  if(!response.ok) throw new Error(data.error||'Unable to save. Please try again.');
  return data as T;
}
