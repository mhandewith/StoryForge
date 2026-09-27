import {test,expect} from '@playwright/test';

test('an admin can open a role to every actor and restrict it again',async({page,request,browser})=>{
 const post=async(path:string,data:unknown)=>{const r=await request.post(path,{data});expect(r.ok()).toBeTruthy();return r.json();};
 const project=await post('/api/projects',{name:'Open role browser project'});
 const actor=await post('/api/actors',{name:'Open role browser actor'});
 await request.put(`/api/actors/${actor.id}/login`,{data:{email:'any-browser@example.test'}});
 const character=await post('/api/characters',{name:'Guest narrator',project_id:project.id});
 const scene=await post('/api/scenes',{name:'Everyone can try',project_id:project.id,position:1});
 await post('/api/events',{project_id:project.id,scene_id:scene.id,character_id:character.id,text:'Anyone can tell this story.',direction:'Warmly',position:1,start_ms:0});
 await page.goto('/');await page.getByLabel('Current project').selectOption(project.id);
 await page.getByRole('button',{name:'Cast & characters'}).click();
 await page.getByLabel('Actor for Guest narrator').selectOption('any');
 await expect(page.getByLabel('Actor for Guest narrator')).toBeEnabled();
 await page.reload();await page.getByLabel('Current project').selectOption(project.id);await page.getByRole('button',{name:'Cast & characters'}).click();
 await expect(page.getByLabel('Actor for Guest narrator')).toHaveValue('any');
 const context=await browser.newContext({baseURL:'http://127.0.0.1:18088',extraHTTPHeaders:{Authorization:`Bearer ${process.env.STORYFORGE_DEV_AUTH_TOKEN}`,'X-StoryForge-Dev-Email':'any-browser@example.test'}});
 const performer=await context.newPage();await performer.goto('/');
 await performer.getByRole('button',{name:/Everyone can try/}).click();
 await expect(performer.locator('.performance-text')).toHaveText('Anyone can tell this story.');
 await page.getByLabel('Actor for Guest narrator').selectOption(actor.id);
 await expect(page.getByLabel('Actor for Guest narrator')).toBeEnabled();
 await context.close();
});
