import {test,expect} from '@playwright/test';
import {randomUUID} from 'node:crypto';

test('admin selects voices and converts a scene; actors can listen but cannot regenerate',async({page,request,browser})=>{
 test.setTimeout(120000);
 const post=async(path:string,data:unknown)=>{const r=await request.post(path,{data});expect(r.ok(),await r.text()).toBeTruthy();return r.json();};
 const project=await post('/api/projects',{name:'Voice changer browser story'});
 const actor=await post('/api/actors',{name:'Voice changer actor'});
 await request.put(`/api/actors/${actor.id}/login`,{data:{email:'voice-browser@example.test'}});
 const scene=await post('/api/scenes',{project_id:project.id,name:'A voiced scene',position:1});
 const characters=[];
 const wolfTakes:string[]=[];
 for(const [i,name] of ['Wolf','Owl'].entries()){
  const character=await post('/api/characters',{project_id:project.id,name});characters.push(character);
  await request.put(`/api/assignments/${character.id}`,{data:{actor_id:actor.id}});
  const line=await post('/api/events',{project_id:project.id,scene_id:scene.id,character_id:character.id,text:`Hello from ${name}.`,direction:'Happily',position:i+1,start_ms:0});
  const wav=Buffer.alloc(32044);wav.write('RIFF',0);wav.writeUInt32LE(32036,4);wav.write('WAVEfmt ',8);wav.writeUInt32LE(16,16);wav.writeUInt16LE(1,20);wav.writeUInt16LE(1,22);wav.writeUInt32LE(16000,24);wav.writeUInt32LE(32000,28);wav.writeUInt16LE(2,32);wav.writeUInt16LE(16,34);wav.write('data',36);wav.writeUInt32LE(32000,40);
  const uploaded=await request.post(`/api/actor/events/${line.id}/takes?revision=1&as_actor=${actor.id}`,{headers:{'Content-Type':'audio/wav','X-Upload-ID':randomUUID().replaceAll('-','')},data:wav});expect(uploaded.ok()).toBeTruthy();
  if(name==='Wolf'){
   wolfTakes.push((await uploaded.json()).id);
   const guest=await post('/api/actors',{name:'Audition guest'});
   await request.put(`/api/assignments/${character.id}`,{data:{actor_id:'any'}});
   const second=await request.post(`/api/actor/events/${line.id}/takes?revision=1&as_actor=${guest.id}`,{headers:{'Content-Type':'audio/wav','X-Upload-ID':randomUUID().replaceAll('-','')},data:wav});expect(second.ok()).toBeTruthy();wolfTakes.push((await second.json()).id);
  }
 }
 await page.goto('/');await page.getByLabel('Current project').selectOption(project.id);
 await page.getByRole('button',{name:'Cast & characters'}).click();
 for(const c of characters){await page.getByLabel(`Target voice for ${c.name}`,{exact:true}).selectOption(c.name==='Owl'?'storyforge:audio-isolation:v1':'voice-'+c.name.toLowerCase());await page.getByRole('button',{name:`Save target voice for ${c.name}`,exact:true}).click();await expect(page.getByRole('button',{name:`Save target voice for ${c.name}`,exact:true})).toBeDisabled();}
 await page.getByRole('button',{name:'Refresh voices for Wolf',exact:true}).click();
 await expect(page.getByLabel('Target voice for Wolf',{exact:true})).toBeEnabled();
 await page.getByRole('button',{name:'Scenes & script'}).click();
 await expect(page.getByRole('button',{name:'Voice scene',exact:true})).toBeEnabled();
 await page.getByText('Converted lines',{exact:true}).click();
 const takePicker=page.getByLabel('Take for line 1',{exact:true});
 await expect(takePicker).toHaveValue(wolfTakes[1]);
 await takePicker.selectOption(wolfTakes[0]);
 const rawPlayer=page.getByLabel('Play selected take for line 1',{exact:true});
 await rawPlayer.evaluate((el:HTMLAudioElement)=>el.play());await expect.poll(()=>rawPlayer.evaluate((el:HTMLAudioElement)=>el.currentTime)).toBeGreaterThan(0);await rawPlayer.evaluate((el:HTMLAudioElement)=>el.pause());
 const statusPath=`/api/actor/scenes/${scene.id}/voicing`;
 expect((await (await request.get(statusPath)).json()).lines[0].take_id).toBe(wolfTakes[1]);
 await page.getByRole('button',{name:'Make selected take preferred for line 1',exact:true}).click();
 await expect.poll(async()=>(await (await request.get(statusPath)).json()).lines[0].take_id).toBe(wolfTakes[0]);
 await expect(takePicker).toBeEnabled();
 await page.getByText('Converted lines',{exact:true}).click();
 page.once('dialog',d=>d.accept());await page.getByRole('button',{name:'Voice scene',exact:true}).click();
 await expect(page.getByLabel('ElevenLabs scene')).toContainText('Scene conversion complete',{timeout:60000});
 const isolationCalls=(await (await request.get('http://127.0.0.1:18089/test/state')).json()).isolations;
 expect(isolationCalls).toBeGreaterThan(0);
 const compiled=page.getByLabel('Play converted scene');await compiled.evaluate((el:HTMLAudioElement)=>el.play());await expect.poll(()=>compiled.evaluate((el:HTMLAudioElement)=>el.currentTime)).toBeGreaterThan(0);await compiled.evaluate((el:HTMLAudioElement)=>el.pause());
 await page.getByText('Converted lines',{exact:true}).click();
 page.once('dialog',d=>d.accept());await page.getByRole('button',{name:'Regenerate line 1',exact:true}).click();
 await expect(page.getByRole('button',{name:'Voice scene',exact:true})).toBeEnabled({timeout:60000});
 expect((await (await request.get('http://127.0.0.1:18089/test/state')).json()).isolations).toBe(isolationCalls);
 page.once('dialog',d=>d.accept());await page.getByRole('button',{name:'Regenerate line 2',exact:true}).click();
 await expect(page.getByRole('button',{name:'Voice scene',exact:true})).toBeEnabled({timeout:60000});
 expect((await (await request.get('http://127.0.0.1:18089/test/state')).json()).isolations).toBe(isolationCalls+1);
 const isolated=page.getByLabel('Converted line 2',{exact:true});
 await isolated.evaluate((el:HTMLAudioElement)=>el.play());
 await expect.poll(()=>isolated.evaluate((el:HTMLAudioElement)=>el.duration)).toBeLessThan(1.2);
 expect(await isolated.evaluate((el:HTMLAudioElement)=>el.duration)).toBeGreaterThan(.9);
 await isolated.evaluate((el:HTMLAudioElement)=>el.pause());
 await page.setViewportSize({width:390,height:844});expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBeTruthy();await page.screenshot({path:'test-results/voicing-admin-mobile.png',fullPage:true});
 const context=await browser.newContext({baseURL:'http://127.0.0.1:18088',extraHTTPHeaders:{Authorization:`Bearer ${process.env.STORYFORGE_DEV_AUTH_TOKEN}`,'X-StoryForge-Dev-Email':'voice-browser@example.test'}});
 const actorPage=await context.newPage();await actorPage.goto('/');await actorPage.getByRole('button',{name:/A voiced scene/}).click();
 await expect(actorPage.getByLabel('Take for line 1',{exact:true})).toHaveCount(0);
 await expect(actorPage.getByLabel('Play converted scene')).toBeVisible();await expect(actorPage.getByLabel('Converted line 1',{exact:true})).toBeVisible();
 await expect(actorPage.getByRole('button',{name:'Voice scene',exact:true})).toHaveCount(0);await expect(actorPage.getByRole('button',{name:/Regenerate line/})).toHaveCount(0);await expect(actorPage.getByRole('button',{name:/preferred/i})).toHaveCount(0);
 const converted=actorPage.getByLabel('Converted line 1',{exact:true});await converted.evaluate((el:HTMLAudioElement)=>el.play());await expect.poll(()=>converted.evaluate((el:HTMLAudioElement)=>el.currentTime)).toBeGreaterThan(0);
 await actorPage.setViewportSize({width:390,height:844});expect(await actorPage.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBeTruthy();await actorPage.screenshot({path:'test-results/voicing-actor-mobile.png',fullPage:true});await context.close();
});
