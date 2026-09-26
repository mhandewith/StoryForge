export type Named = {id:string;name:string};
export type Scene = Named & {project_id:string;position:number};
export type Character = Named & {project_id:string;target_voice:string};
export type Line = {id:string;project_id:string;scene_id:string;character_id:string;text:string;direction:string;position:number;start_ms:number;revision:number};
export type Workspace = {projects:Named[];actors:Named[];scenes:Scene[];characters:Character[];assignments:{character_id:string;actor_id:string}[];events:Line[]};
export const empty:Workspace={projects:[],actors:[],scenes:[],characters:[],assignments:[],events:[]};
export async function request<T>(path:string,method='GET',body?:unknown):Promise<T> {
  const response=await fetch(path,{method,headers:body===undefined?{}:{'Content-Type':'application/json'},body:body===undefined?undefined:JSON.stringify(body)});
  if(!response.headers.get('content-type')?.includes('application/json'))throw new Error('Your login may have expired. Save or download any local recording before signing in again.');
  const data=await response.json();
  if(!response.ok) throw new Error(data.error||'Unable to save. Please try again.');
  return data as T;
}
