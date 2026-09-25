import { test, expect } from '@playwright/test';

test('prepare a family scene, edit it, and reload it from PostgreSQL', async ({page})=>{
  const pageErrors:string[]=[];
  page.on('pageerror',e=>pageErrors.push(e.message));
  await page.goto('/');
  await expect(page.getByRole('heading',{level:1})).not.toHaveText('Every story starts here.');
  await page.getByText('Create a new project',{exact:true}).click();
  await page.getByLabel('Project name',{exact:true}).fill('The lantern in the woods');
  await page.getByRole('button',{name:'Create project',exact:true}).click();
  await expect(page.getByRole('heading',{level:1})).toHaveText('The lantern in the woods');
  await page.getByRole('button',{name:'Cast & characters'}).click();
  for(const name of ['Hazel','Hannah']){
    await page.getByLabel('Actor name',{exact:true}).fill(name);
    await page.getByRole('button',{name:'Add actor',exact:true}).click();
    await expect(page.getByLabel('Actor name',{exact:true})).toHaveValue('');
    await page.getByLabel('Character name',{exact:true}).fill(name);
    await page.getByRole('button',{name:'Add character',exact:true}).click();
    await expect(page.getByLabel('Character name',{exact:true})).toHaveValue('');
    await page.getByLabel(`Actor for ${name}`).selectOption({label:name});
    await expect(page.getByLabel(`Actor for ${name}`)).toBeEnabled();
  }
  await page.getByRole('button',{name:'Scenes & script'}).click();
  await page.getByLabel('Scene name',{exact:true}).fill('A light between the trees');
  await page.getByRole('button',{name:'Add scene',exact:true}).click();
  await expect(page.getByRole('heading',{name:'A light between the trees',exact:true})).toBeVisible();
  const words=[
    'Did you see that little light?', 'There, just beyond the old oak.',
    'Maybe someone left a lantern.', 'Or maybe the forest is waking up.',
    'Should we follow it?', 'Only if we stay together.',
    'I can hear something singing.', 'It sounds like our song.',
    'Then let’s find out who is there.', 'Ready? Take my hand.',
  ];
  for(let i=0;i<10;i++){
    await page.getByLabel('Character',{exact:true}).selectOption({label:i%2?'Hannah':'Hazel'});
    await page.getByLabel('Dialogue',{exact:true}).fill(words[i]);
    await page.getByLabel('Performance direction').fill(i===0?'Quietly, with wonder':'');
    await page.getByRole('button',{name:'Add line',exact:true}).click();
    await expect(page.getByLabel('Dialogue',{exact:true})).toHaveValue('');
  }
  await expect(page.locator('article.dialogue')).toHaveCount(10);
  await page.getByRole('button',{name:'Edit line 1',exact:true}).click();
  await page.getByLabel('Dialogue',{exact:true}).fill('Did you see that tiny golden light?');
  await page.getByRole('button',{name:'Save changes',exact:true}).click();
  await expect(page.getByText('Did you see that tiny golden light?',{exact:true})).toBeVisible();
  await page.reload();
  await page.getByLabel('Current project').selectOption({label:'The lantern in the woods'});
  await expect(page.locator('article.dialogue')).toHaveCount(10);
  await expect(page.getByText('Did you see that tiny golden light?',{exact:true})).toBeVisible();
  await page.setViewportSize({width:1440,height:1080});
  await page.screenshot({path:'test-results/studio-desktop.png',fullPage:true});
  await page.setViewportSize({width:390,height:844});
  await expect(page.getByRole('button',{name:'Add line',exact:true})).toBeVisible();
  expect(await page.evaluate(()=>document.documentElement.scrollWidth<=window.innerWidth)).toBeTruthy();
  await page.screenshot({path:'test-results/studio-mobile.png',fullPage:true});
  expect(pageErrors).toEqual([]);
});
