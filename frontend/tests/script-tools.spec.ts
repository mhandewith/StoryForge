import {test, expect, type Locator, type Page} from '@playwright/test';

async function drag(page:Page, from:Locator, to:Locator) {
  await from.scrollIntoViewIfNeeded();
  const a=await from.boundingBox(), b=await to.boundingBox();
  if(!a||!b)throw new Error('Missing drag handle');
  await page.mouse.move(a.x+a.width/2,a.y+a.height/2);
  await page.mouse.down();
  await page.mouse.move(a.x+a.width/2+10,a.y+a.height/2,{steps:3});
  await page.mouse.move(b.x+b.width/2,b.y+b.height/2,{steps:12});
  await page.mouse.up();
}

test('import, reorder, edit, and safely remove a script and its cast',async({page})=>{
  test.setTimeout(90000);
  const errors:string[]=[];
  page.on('pageerror',e=>errors.push(e.message));
  await page.goto('/');
  await page.getByRole('button',{name:'Import script',exact:true}).click();
  await page.getByLabel('Tagged script').fill('[script: Broken]\n[scene: Start]\nMissing character');
  await page.getByRole('button',{name:'Preview import'}).click();
  await expect(page.getByRole('alert')).toContainText('Line 3');
  const source=`[script: Browser import]
[cast: Fox | Browser reader]
[cast: Owl | Browser reader]
[cast: Silent]
[scene: Dawn]
[Fox]
[direction: With wonder]
First light.
Through the trees.
[Owl]
Second voice.
[Fox]
Third line.
[scene: Dusk]
[Owl]
Goodnight.`;
  await page.getByLabel('Tagged script').fill(source);
  await page.getByRole('button',{name:'Preview import'}).click();
  await expect(page.getByText('2 scenes · 3 characters · 4 dialogue lines')).toBeVisible();
  await expect(page.getByRole('heading',{level:1})).not.toHaveText('Browser import');
  await page.getByRole('button',{name:'Import as new script'}).click();
  await expect(page.getByRole('heading',{level:1})).toHaveText('Browser import');
  await expect(page.locator('article.dialogue')).toHaveCount(3);
  await expect(page.locator('article.dialogue').first()).toContainText('Through the trees.');

  // Real pointer drag on a number must reorder, without entering edit mode.
  await drag(page,page.getByRole('button',{name:'Edit or move line 1',exact:true}),page.getByRole('button',{name:'Edit or move line 3',exact:true}));
  await expect(page.locator('article.dialogue .dialogue-text')).toHaveText(['Second voice.','Third line.','First light.\nThrough the trees.']);
  await expect(page.getByRole('heading',{name:'Add dialogue',exact:true})).toBeVisible();
  await page.getByRole('button',{name:'Edit or move line 1',exact:true}).click();
  await expect(page.getByRole('heading',{name:'Edit dialogue',exact:true})).toBeVisible();
  await page.getByLabel('Dialogue',{exact:true}).fill('Edited second voice.');
  await page.getByRole('button',{name:'Save changes'}).click();
  await expect(page.locator('article.dialogue').first()).toContainText('Edited second voice.');

  // Keyboard and pointer scene moves both persist; selected scene stays selected.
  const sceneHandle=page.getByRole('button',{name:'Select or move scene 1',exact:true});
  await sceneHandle.focus();
  await page.keyboard.press('Space');
  await page.keyboard.press('ArrowDown');
  await page.keyboard.press('Space');
  await expect(page.locator('.scene-title')).toHaveText(['Dusk1 lines','Dawn3 lines']);
  await expect(page.locator('.script-heading h2')).toHaveText('Dawn');
  await drag(page,page.getByRole('button',{name:'Select or move scene 2',exact:true}),page.getByRole('button',{name:'Select or move scene 1',exact:true}));
  await expect(page.locator('.scene-title')).toHaveText(['Dawn3 lines','Dusk1 lines']);
  await page.reload();
  await page.getByLabel('Current project').selectOption({label:'Browser import'});
  await expect(page.locator('article.dialogue').first()).toContainText('Edited second voice.');
  await expect(page.locator('.scene-title')).toHaveText(['Dawn3 lines','Dusk1 lines']);
  await page.screenshot({path:'test-results/script-tools-desktop.png',fullPage:true});
  await page.setViewportSize({width:390,height:844});
  expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBeTruthy();
  await page.screenshot({path:'test-results/script-tools-mobile.png',fullPage:true});
  await page.setViewportSize({width:1440,height:1080});

  await page.getByRole('button',{name:'Delete line 1',exact:true}).click();
  await expect(page.locator('article.dialogue')).toHaveCount(3);
  await page.getByRole('button',{name:'Cancel deleting line 1',exact:true}).click();
  await page.getByRole('button',{name:'Delete line 1',exact:true}).click();
  await page.getByRole('button',{name:'Confirm delete line 1',exact:true}).click();
  await expect(page.locator('article.dialogue')).toHaveCount(2);
  await page.getByRole('button',{name:'Remove scene',exact:true}).click();
  await expect(page.getByRole('dialog')).toContainText('all 2 of its dialogue lines');
  await page.getByRole('button',{name:'Cancel',exact:true}).click();
  await expect(page.locator('article.dialogue')).toHaveCount(2);
  await page.getByRole('button',{name:'Remove scene',exact:true}).click();
  await page.getByRole('button',{name:'Confirm removal'}).click();
  await expect(page.locator('.scene-row')).toHaveCount(1);
  await expect(page.locator('article.dialogue')).toHaveCount(1);

  await page.getByRole('button',{name:'Cast & characters'}).click();
  await expect(page.getByRole('button',{name:'Remove character Owl',exact:true})).toBeDisabled();
  await page.getByRole('button',{name:'Remove actor Browser reader',exact:true}).click();
  await page.getByRole('button',{name:'Cancel',exact:true}).click();
  await expect(page.getByLabel('Actor for Owl')).not.toHaveValue('');
  await page.getByRole('button',{name:'Remove actor Browser reader',exact:true}).click();
  await page.getByRole('button',{name:'Confirm removal'}).click();
  await expect(page.getByLabel('Actor for Owl')).toHaveValue('');
  await expect(page.getByLabel('Actor for Fox')).toHaveValue('');
  await page.getByRole('button',{name:'Remove character Silent',exact:true}).click();
  await page.getByRole('button',{name:'Confirm removal'}).click();
  await expect(page.getByRole('button',{name:'Remove character Silent',exact:true})).toHaveCount(0);
  await page.getByRole('button',{name:'Delete script',exact:true}).click();
  await expect(page.getByRole('dialog')).toContainText('archived in the database');
  await page.getByRole('button',{name:'Cancel',exact:true}).click();
  await expect(page.getByRole('heading',{level:1})).toHaveText('Browser import');
  await page.getByRole('button',{name:'Delete script',exact:true}).click();
  await page.getByRole('button',{name:'Confirm removal'}).click();
  await expect(page.getByLabel('Current project').locator('option').filter({hasText:'Browser import'})).toHaveCount(0);
  expect(errors).toEqual([]);
});
