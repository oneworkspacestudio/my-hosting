import {chromium} from '@playwright/test';
import {execFileSync} from 'node:child_process';
import {randomBytes} from 'node:crypto';
import {mkdir,readFile,writeFile} from 'node:fs/promises';
import path from 'node:path';

// This script deliberately cannot target a real Workspace installation.
const root=process.env.MYHOSTING_TEST_ROOT;
if(!root||!root.replaceAll('\\','/').startsWith('C:/Projects/.cache/myhosting-acceptance-20261018/'))throw new Error('Provide the dedicated synthetic NTFS fixture root.');
const base='http://127.0.0.1:19007';
const work=path.resolve(process.env.MYHOSTING_TEST_OUTPUT||'../../tooling/my-hosting-work');
const capture=path.resolve('../docs/screenshots');
await mkdir(capture,{recursive:true});await mkdir(work,{recursive:true});
const powershell='C:/Program Files/PowerShell/7/pwsh.exe';
const psQuote=s=>`'${s.replaceAll("'","''")}'`;
function decrypt(file){return execFileSync(powershell,['-NoProfile','-Command',`Add-Type -AssemblyName System.Security; $b=[IO.File]::ReadAllBytes(${psQuote(file)}); $p=[Security.Cryptography.ProtectedData]::Unprotect($b,$null,[Security.Cryptography.DataProtectionScope]::LocalMachine); [Console]::Write([Text.Encoding]::UTF8.GetString($p))`],{encoding:'utf8',windowsHide:true});}
function protectFile(file,value){execFileSync(powershell,['-NoProfile','-Command',`Add-Type -AssemblyName System.Security; $s=[Console]::In.ReadToEnd(); $b=[Text.Encoding]::UTF8.GetBytes($s); $p=[Security.Cryptography.ProtectedData]::Protect($b,$null,[Security.Cryptography.DataProtectionScope]::LocalMachine); [IO.File]::WriteAllBytes(${psQuote(file)},$p)`],{input:value,windowsHide:true});}
const context=await chromium.launchPersistentContext(path.join(root,'browser-profile'),{headless:true,executablePath:'C:/Program Files/Google/Chrome/Application/chrome.exe',viewport:{width:1440,height:1050},args:['--disk-cache-size=1','--media-cache-size=1'],acceptDownloads:true,downloadsPath:path.join(root,'downloads')});
const page=context.pages()[0]||await context.newPage();const errors=[];page.on('pageerror',e=>errors.push(e.message));const checks=[];
const check=(name,condition)=>{if(!condition)throw new Error(name);checks.push(name);console.log('PASS',name);};
try{
 const status=await fetch(base+'/api/bootstrap-status').then(r=>r.json());
 if(!status.configured){const token=decrypt(path.join(root,'secrets','setup.dpapi'));const password=randomBytes(18).toString('base64url');protectFile(path.join(root,'secrets','ui-test-login.dpapi'),JSON.stringify({email:'hosting-tester@example.test',password}));await page.goto(`${base}/#setup/${token}`);await page.locator('input[name=setupEmail]').fill('hosting-tester@example.test');await page.locator('input[name=setupPassword]').fill(password);await page.locator('input[name=setupConfirm]').fill(password);await page.getByRole('button',{name:'Create administrator & open Hosting'}).click();}
 else{await page.goto(base);if(await page.locator('input[name=loginEmail]').isVisible()){const credentials=JSON.parse(decrypt(path.join(root,'secrets','ui-test-login.dpapi')));await page.locator('input[name=loginEmail]').fill(credentials.email);await page.locator('input[name=loginPassword]').fill(credentials.password);await page.getByRole('button',{name:'Sign in',exact:true}).click();}}
 await page.getByRole('heading',{name:'Your projects',exact:true}).waitFor();check('Standalone browser setup/login reaches real projects dashboard',true);
 const me=await page.request.get(base+'/api/me').then(r=>r.json());check('Real owner session and CSRF token',me.user.role==='owner'&&me.csrf.length>20);
 await page.screenshot({path:path.join(capture,'projects-empty.png'),fullPage:true});
 const projectName='My Hosting acceptance '+Date.now();const slug='acceptance-'+Date.now();await page.getByRole('button',{name:'Add new project'}).click();await page.locator('input[name=name]').fill(projectName);await page.locator('input[name=slug]').fill(slug);await page.getByRole('button',{name:'Continue to build settings'}).click();await page.locator('input[name=output]').fill('.');await page.getByRole('button',{name:'Review project'}).click();await page.getByRole('button',{name:'Create project',exact:true}).click();await page.getByRole('heading',{name:projectName,exact:true}).waitFor();check('Three-step project wizard persists an actual static project',true);
 const projects=await page.request.get(base+'/api/projects').then(r=>r.json());const project=projects.find(p=>p.slug===slug);check('Created project is returned by server',!!project?.id);
 await page.screenshot({path:path.join(capture,'project-overview.png'),fullPage:true});
 for(const [tab,label]of[['build','Build settings'],['domains','Domains & HTTPS'],['process','Logs & processes'],['access','Access'],['audit','Audit']]){await page.goto(`${base}/#project/${project.id}/${tab}`);await page.getByRole('link',{name:label,exact:true}).waitFor();await page.waitForTimeout(200);const alert=page.getByRole('alert');check(`Working ${tab} screen without API errors`,await alert.count()===0);}
 await page.goto(`${base}/#settings`);await page.getByRole('heading',{name:'Settings & health',exact:true}).waitFor();await page.screenshot({path:path.join(capture,'settings.png'),fullPage:true});
 await page.goto(base+'/#projects');await page.getByRole('heading',{name:'Your projects',exact:true}).waitFor();await page.getByRole('button',{name:'Toggle color theme'}).click();await page.screenshot({path:path.join(capture,'projects-dark.png'),fullPage:true});check('Dark theme switches actual design tokens',await page.locator('html').getAttribute('data-theme')==='dark');await page.getByRole('button',{name:'Toggle color theme'}).click();
 await page.setViewportSize({width:390,height:844});await page.screenshot({path:path.join(capture,'projects-mobile.png'),fullPage:true});check('Mobile has no horizontal page overflow',await page.evaluate(()=>document.documentElement.scrollWidth<=window.innerWidth));
 check('No uncaught browser errors',errors.length===0);
 await writeFile(path.join(work,'ui-first-acceptance.json'),JSON.stringify({checks,errors,fixture:'isolated synthetic standalone',projectId:project.id,slug},null,2));console.log(JSON.stringify({checks:checks.length,errors:errors.length}));
}finally{await context.close();}
