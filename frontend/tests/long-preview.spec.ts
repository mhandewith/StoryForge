import {test,expect} from '@playwright/test';
import {randomUUID} from 'node:crypto';

test('long previews build ordered listening parts without paid conversions',async({page,request})=>{
 test.setTimeout(180000);
 const imported=await request.post('/api/import',{data:{text:'[script: Long preview test]\n[cast: Reader | Long preview actor]\n[scene: One long scene]\n'+Array.from({length:125},(_,i)=>`[Reader]\nLine ${i+1}.`).join('\n'),request_id:randomUUID().replaceAll('-','')}});
 expect(imported.ok(),await imported.text()).toBeTruthy();
 const project=await imported.json();
 await page.goto('/');await page.getByLabel('Current project').selectOption(project.id);
 await page.getByRole('button',{name:'Generate scene preview',exact:true}).click();
 const preview=page.getByLabel('Whole scene preview');
 await expect(preview.getByRole('heading',{name:'Part 4 of 4 · Lines 121–125'})).toBeVisible({timeout:150000});
 await expect(preview.locator('audio')).toHaveCount(4);
 await expect(preview.getByRole('heading',{name:'Part 1 of 4 · Lines 1–40'})).toBeVisible();
 const player=page.getByLabel('Play scene part 4');
 await player.evaluate((el:HTMLAudioElement)=>el.play());
 await expect.poll(()=>player.evaluate((el:HTMLAudioElement)=>el.currentTime)).toBeGreaterThan(0);
 await player.evaluate((el:HTMLAudioElement)=>el.pause());
 const workspace=await (await request.get('/api/workspace')).json();
 const scene=workspace.scenes.find((s:{project_id:string})=>s.project_id===project.id);
 const endpoint=`/api/actor/scenes/${scene.id}/preview`;
 expect((await request.post(endpoint,{data:{part:4}})).status()).toBe(400);
 expect((await request.post(endpoint,{data:{part:1,snapshot:'outdated'}})).status()).toBe(409);
 const status=await (await request.get(`/api/actor/scenes/${scene.id}/voicing`)).json();
 expect(status.run).toBeNull();
});
