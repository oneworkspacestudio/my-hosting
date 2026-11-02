import {test} from 'node:test';
import assert from 'node:assert/strict';
import {createServer} from 'node:http';
import {readFile} from 'node:fs/promises';
import {resolve,extname} from 'node:path';
import {chromium} from '@playwright/test';

test('My Git wizard resolves names and both URL forms before review',async()=>{
 const root=resolve(import.meta.dirname,'../../internal/webui/dist');let received=[],created;
 const server=createServer(async(req,res)=>{
  const json=(value,status=200)=>{res.writeHead(status,{'content-type':'application/json'});res.end(JSON.stringify(value));};
  if(req.url.startsWith('/api/')){
   if(req.url==='/api/bootstrap-status')return json({});
   if(req.url==='/api/me')return json({user:{id:'synthetic',name:'Synthetic tester',role:'owner'},csrf:'synthetic-csrf'});
   if(req.url==='/api/repositories/resolve'){
    let body='';for await(const chunk of req)body+=chunk;const input=JSON.parse(body);received.push(input.repository);
    if(input.repository==='ambiguous')return json({error:'Several accessible repositories have that name; enter owner/repository to choose one'},409);
    return json({provider:'mygit',repository:'https://workspace.example.test/git/synthetic/site',defaultBranch:'trunk',private:true,detection:{adapter:'static',install:[],build:[],start:[],output:'.',message:'Static files detected.'}});
   }
   if(req.url==='/api/projects'&&req.method==='POST'){let body='';for await(const chunk of req)body+=chunk;created=JSON.parse(body);return json({...created,id:'fixture-project'});}
   if(req.url==='/api/projects')return json([]);
   if(req.url==='/api/projects/fixture-project')return json({...created,id:'fixture-project',role:'owner'});
   if(req.url.endsWith('/metrics'))return json({requests:0,serverErrors:0});
   return json({deployments:[]});
  }
  try{const file=resolve(root,'.'+(req.url==='/'?'/index.html':req.url.split('?')[0]));assert.ok(file.startsWith(root));const bytes=await readFile(file);res.writeHead(200,{'content-type':({'.html':'text/html','.js':'text/javascript','.css':'text/css','.svg':'image/svg+xml'})[extname(file)]||'application/octet-stream'});res.end(bytes);}catch{res.writeHead(404);res.end();}
 });
 await new Promise(r=>server.listen(0,'127.0.0.1',r));
 let browser;
 try{
  browser=await chromium.launch({headless:true,executablePath:process.env.HOSTING_TEST_BROWSER||'C:/Program Files/Google/Chrome/Application/chrome.exe'});
  const url=`http://127.0.0.1:${server.address().port}/`;
  for(const [width,input] of [[1280,'site'],[390,'https://workspace.example.test/git/synthetic/site.git'],[1280,'https://workspace.example.test/git/synthetic/site']]){
   const page=await browser.newPage({viewport:{width,height:900}});await page.goto(url+'#new');
   await page.locator('[name=name]').fill('Synthetic site');await page.locator('[name=slug]').fill('synthetic-site');await page.locator('[name=repository]').fill(input);
   await page.getByRole('button',{name:'Continue to build settings →'}).click();await page.getByRole('heading',{name:'How should this project run?'}).waitFor();
   assert.equal(await page.locator('[name=install]').inputValue(),'[]');assert.equal(await page.locator('[name=output]').inputValue(),'.');
   await page.getByRole('button',{name:/Review/}).click();await page.getByRole('heading',{name:'Ready for its first deployment'}).waitFor();
   if(process.env.HOSTING_ACCEPTANCE_SCREENSHOTS)await page.screenshot({path:process.env.HOSTING_ACCEPTANCE_SCREENSHOTS+`/wizard-${width}.png`,fullPage:true});
   assert.match(await page.locator('body').innerText(),/trunk/);await page.locator('[name=trusted]').check();await page.getByRole('button',{name:'Create project',exact:true}).click();await page.waitForURL('**/#project/fixture-project/overview');
   assert.equal(created.branch,'trunk');assert.equal(created.repository,'https://workspace.example.test/git/synthetic/site');assert.equal(created.trusted,true);await page.close();
  }
  assert.equal(received.length,3);
  const page=await browser.newPage();await page.goto(url+'#new');await page.locator('[name=name]').fill('Ambiguous');await page.locator('[name=slug]').fill('ambiguous-site');await page.locator('[name=repository]').fill('ambiguous');await page.getByRole('button',{name:'Continue to build settings →'}).click();await page.getByRole('alert').filter({hasText:'Several accessible repositories'}).waitFor();assert.equal(await page.locator('[name=repository]').count(),1);assert.equal(await page.getByRole('heading',{name:'Ready for its first deployment'}).count(),0);await page.close();
 }finally{if(browser)await browser.close();await new Promise(r=>server.close(r));}
});
