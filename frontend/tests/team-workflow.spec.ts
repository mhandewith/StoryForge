import {test,expect} from '@playwright/test';

test('team countdown, safe Nailed it retry, and actor handoffs follow script order',async({page,request})=>{
 test.setTimeout(90000);
 const post=async(path:string,data:unknown)=>{const r=await request.post(path,{data});expect(r.ok(),await r.text()).toBeTruthy();return r.json();};
 const project=await post('/api/projects',{name:'Team workflow story'});
 const scene=await post('/api/scenes',{name:'Team handoff scene',project_id:project.id,position:1});
 const actors=[];const lines=[];
 for(const name of ['Team Hannah','Team Hazel'])actors.push(await post('/api/actors',{name}));
 for(const [i,name] of ['Fox','Owl','Guest'].entries()){
  const c=await post('/api/characters',{project_id:project.id,name});
  expect((await request.put(`/api/assignments/${c.id}`,{data:{actor_id:i<2?actors[i].id:'any'}})).ok()).toBeTruthy();
  lines.push(await post('/api/events',{project_id:project.id,scene_id:scene.id,character_id:c.id,text:`Team line ${i+1}.`,direction:'With excitement',position:i+1,start_ms:0}));
 }
 await page.context().grantPermissions(['microphone']);
 await page.goto('/');await page.getByRole('button',{name:'Recording studio',exact:true}).click();
 await page.getByRole('button',{name:'Team mode',exact:true}).click();
 await page.getByLabel('Team name',{exact:true}).fill('Together team');
 for(const a of actors)await page.getByRole('checkbox',{name:a.name,exact:true}).check();
 await page.getByRole('button',{name:'Save team and record',exact:true}).click();
 await page.getByRole('button',{name:/Team handoff scene/}).click();
 await expect(page.locator('.performance-label')).toContainText('Team Hannah as Fox');
 await page.getByRole('button',{name:'● Record',exact:true}).click();
 await expect(page.locator('.recording-state')).toContainText('Starting in 3');
 await page.getByRole('button',{name:'Cancel countdown',exact:true}).click();
 await expect(page.locator('.recording-state')).toContainText('Ready when you are');
 await expect(page.getByRole('button',{name:'■ Stop recording'})).toHaveCount(0);
 await page.getByRole('button',{name:'● Record',exact:true}).click();
 await expect(page.locator('.recording-state')).toContainText('Starting in 3');
 await expect(page.locator('.recording-state')).toContainText('Starting in 2');
 await expect(page.locator('.recording-state')).toContainText('Starting in 1');
 await expect(page.locator('.recording-state')).toContainText('0:01',{timeout:10000});
 await page.route('**/api/actor/events/*/takes?*',route=>route.fulfill({status:503,contentType:'application/json',body:'{"error":"Temporary upload failure"}'}),{times:1});
 await page.getByRole('button',{name:'Nailed it',exact:true}).click();
 await expect(page.getByRole('alert')).toContainText('Temporary upload failure');
 await expect(page.getByLabel('Listen to your new take')).toBeVisible();
 await expect(page.locator('.performance-text')).toHaveText('Team line 1.');
 await page.getByRole('button',{name:'Nailed it',exact:true}).click();
 await expect(page.locator('.performance-text')).toHaveText('Team line 2.');
 await expect(page.locator('.performance-label')).toContainText('Team Hazel as Owl');
 await page.setViewportSize({width:390,height:844});
 expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBeTruthy();
 await page.screenshot({path:'test-results/team-studio-mobile.png',fullPage:true});
 for(let i=1;i<3;i++){
  if(i===2){await page.getByLabel('Team performer',{exact:true}).selectOption(actors[1].id);await expect(page.locator('.performance-label')).toContainText('Team Hazel as Guest');}
  await page.getByRole('button',{name:'● Record',exact:true}).click();
  await expect(page.locator('.recording-state')).toContainText('0:01',{timeout:10000});
  await page.getByRole('button',{name:'Nailed it',exact:true}).click();
  if(i===1)await expect(page.locator('.performance-text')).toHaveText('Team line 3.');
 }
 await expect(page.getByRole('button',{name:/Team handoff scene/})).toContainText('All lines have a take');
 const takes=await (await request.get(`/api/actor/takes?scene_id=${scene.id}`)).json();
 expect(takes).toHaveLength(3);
 for(const [i,l] of lines.entries())expect(takes.find((t:{event_id:string})=>t.event_id===l.id).actor_id).toBe(actors[i===0?0:1].id);
 await page.getByRole('button',{name:'Change team',exact:true}).click();
 await page.getByLabel('Saved team',{exact:true}).selectOption({label:'Together team'});
 for(const a of actors)await expect(page.getByRole('checkbox',{name:a.name,exact:true})).toBeChecked();
});
