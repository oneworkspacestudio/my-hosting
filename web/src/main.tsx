import React,{useCallback,useEffect,useMemo,useRef,useState} from 'react';
import {createRoot} from 'react-dom/client';
import {Fields} from './components/ui';
import {Shell} from './generated/Shell';
import {Projects} from './generated/Projects';
import {Create} from './generated/Create';
import {Project} from './generated/Project';
import {Deploy} from './generated/Deploy';
import {Settings} from './generated/Settings';
import {Team} from './generated/Team';
import {Login} from './generated/Login';
import {Audit} from './generated/Audit';
import {Setup} from './generated/Setup';
import './styles.css';

const base=location.pathname.replace(/\/$/,'');
const logo=`${base}/my-hosting.svg`;
const options=(xs:string[])=>xs.map(value=>({value,label:value}));
const adapters=[{value:'static',label:'Static site · HTML / Vite / React output'},{value:'node',label:'Node.js · Express / Nest / custom server'},{value:'next',label:'Next.js SSR · standalone output'},{value:'nuxt',label:'Nuxt / Nitro · node-server output'}];
const defaults={name:'',slug:'',repository:'',branch:'main',adapter:'static',nodeMajor:'24',install:'[]',build:'[]',start:'[]',output:'.',baseDirectory:'.',healthPath:'/',timeout:'600',memoryMB:'1024',diskMB:'1024',previews:true,trusted:false,deployEnvironment:'production',uploadEnvironment:'production',deployBranch:'main',deployCommit:'',clearCache:false,secretEnvironment:'production',runtime:'caddy',memberRole:'viewer',globalRole:'viewer',concurrency:'1',retain:'10',caddyPort:'9080',public:false,addressStyle:'nested',transport:'direct',previewDomain:'',deployFilter:'all'};
const roleRank=(s:string)=>(({owner:4,admin:3,developer:2,viewer:1} as Record<string,number>)[s]||0);
const tone=(s:string)=>s==='published'||s==='ready'?'success':s==='failed'||s==='crash-loop'?'error':['queued','preparing','uploading','building','publishing','restarting'].includes(s)?'info':s==='cancelled'?'warning':'muted';
const formatDate=(s:string)=>s?new Date(s).toLocaleString(undefined,{dateStyle:'medium',timeStyle:'short'}):'Not started';
const bytes=(n:number)=>n>=1048576?`${(n/1048576).toFixed(1)} MiB`:n?`${(n/1024).toFixed(1)} KiB`:'—';
const deployHref=(pid:string,id:string)=>`#project/${pid}/deploy/${id}`;
const changeRoute=(s:string)=>{location.hash=s;};
const parseArgs=(raw:string)=>{const v=JSON.parse(raw||'[]');if(!Array.isArray(v)||v.some(x=>typeof x!=='string'))throw new Error('Commands must be JSON arrays of strings. Example: ["npm","run","build"]');return v;};
const filePick=()=>new Promise<File|null>(resolve=>{const input=document.createElement('input');input.type='file';input.accept='.zip,application/zip';input.onchange=()=>resolve(input.files?.[0]||null);input.oncancel=()=>resolve(null);input.click();});
const downloadBlob=(blob:Blob,name:string)=>{const url=URL.createObjectURL(blob);const a=document.createElement('a');a.href=url;a.download=name;a.click();setTimeout(()=>URL.revokeObjectURL(url),10000);};

function Application(){
 const setupToken=useRef(location.hash.startsWith('#setup/')?location.hash.slice(7):'');
 const [route,setRoute]=useState(location.hash.slice(1)||'projects');
 const [user,setUser]=useState<any>(null),[boot,setBoot]=useState<any>({}),[loading,setLoading]=useState(true);
 const csrf=useRef('');
 const [fields,setFields]=useState<Record<string,any>>({...defaults});
 const [error,setError]=useState(''),[notice,setNotice]=useState(''),[busy,setBusy]=useState('');
 const [projects,setProjects]=useState<any[]>([]),[project,setProject]=useState<any>(null),[deployments,setDeployments]=useState<any[]>([]),[deploy,setDeploy]=useState<any>(null);
 const [domains,setDomains]=useState<any[]>([]),[domainsInfo,setDomainsInfo]=useState<any>({}),[secrets,setSecrets]=useState<any[]>([]),[github,setGithub]=useState<any>({}),[webhookSecret,setWebhookSecret]=useState('');
 const [members,setMembers]=useState<any[]>([]),[team,setTeam]=useState<any[]>([]),[teamGuidance,setTeamGuidance]=useState(''),[audit,setAudit]=useState<any[]>([]);
 const [settings,setSettings]=useState<any>({settings:defaults,runtimes:[]}),[metrics,setMetrics]=useState<any>({requests:0,serverErrors:0,scope:'No access log observations yet.'});
 const [process,setProcess]=useState<any>({state:'not loaded'}),[processLogs,setProcessLogs]=useState(''),[buildLogs,setBuildLogs]=useState(''),[streamStatus,setStreamStatus]=useState('Waiting for log stream');
 const [step,setStep]=useState(0),[showTrigger,setShowTrigger]=useState(false),[offset,setOffset]=useState(0),[epoch,setEpoch]=useState(0),[notifications,setNotifications]=useState(false);
 const mounted=useRef(true),routeRef=useRef(route);routeRef.current=route;
 const api=useCallback(async(path:string,method='GET',body?:any,raw=false)=>{
   const headers:Record<string,string>={};if(method!=='GET')headers['X-CSRF-Token']=csrf.current;
   if(body instanceof File||body instanceof Blob)headers['Content-Type']='application/zip';else if(body!==undefined)headers['Content-Type']='application/json';
   const response=await fetch(`${base}/api/${path}`,{method,headers,body:body===undefined?undefined:body instanceof Blob?body:JSON.stringify(body),credentials:'same-origin'});
   if(response.status===401){setUser(null);throw new Error('Your Hosting session expired. Open My Hosting from Workspace to reconnect.');}
   if(!response.ok){const data=await response.json().catch(()=>({error:`Request failed (HTTP ${response.status}). Please retry.`}));throw new Error(data.error||`Request failed (HTTP ${response.status})`);}
   return raw?response.blob():response.json();
 },[]);
 const refresh=()=>setEpoch(x=>x+1);
 useEffect(()=>{if(setupToken.current)history.replaceState(null,'',`${base}/#setup`);const hash=()=>{setRoute(location.hash.slice(1)||'projects');setError('');setNotice('');setShowTrigger(false);setOffset(0);};window.addEventListener('hashchange',hash);const saved=localStorage.getItem('myhosting-theme');if(saved)document.documentElement.dataset.theme=saved;return()=>{window.removeEventListener('hashchange',hash);mounted.current=false;};},[]);
 useEffect(()=>{(async()=>{try{const b=await fetch(`${base}/api/bootstrap-status`).then(r=>r.json());setBoot(b);const response=await fetch(`${base}/api/me`);if(response.status===401){if(b.oidc&&!sessionStorage.getItem('myhosting-auth-tried')){sessionStorage.setItem('myhosting-auth-tried','1');location.assign(`${base}/auth/login`);}return;}if(!response.ok)throw new Error('Hosting is unavailable. Check the application process and reload.');const me=await response.json();csrf.current=me.csrf;setUser(me.user);if(me.publication)setSettings(x=>({...x,settings:{...x.settings,...me.publication}}));sessionStorage.removeItem('myhosting-auth-tried');}catch(e:any){setError(e.message);}finally{setLoading(false);}})();},[]);
 const suggestedSiteURL=settings.settings?.public&&settings.settings?.previewDomain?`https://${fields.slug||'my-site'}${settings.settings.addressStyle==='flat'?'-sites':''}.${settings.settings.previewDomain}`:`http://${fields.slug||'my-site'}.localhost:${settings.settings?.caddyPort||9080}`;
 const routeParts=route.split('/'),pid=routeParts[0]==='project'?routeParts[1]:'',tab=routeParts[2]||'overview',did=tab==='deploy'?routeParts[3]:'';
 const globalAdmin=roleRank(user?.role)>=3,canAdmin=globalAdmin||roleRank(project?.role)>=3,canDeploy=canAdmin||roleRank(project?.role)>=2;
 const fillProject=(p:any)=>setFields(f=>({...f,...p,nodeMajor:String(p.nodeMajor),timeout:String(p.timeout),memoryMB:String(p.memoryMB),diskMB:String(p.diskMB),install:JSON.stringify(p.install||[]),build:JSON.stringify(p.build||[]),start:JSON.stringify(p.start||[]),deployBranch:p.branch}));
 useEffect(()=>{if(!user)return;let alive=true;setBusy('Loading your Hosting workspace…');(async()=>{
  try{
   const ps=await api('projects');if(!alive)return;setProjects(ps);
   if(pid){const p=await api(`projects/${pid}`);if(!alive)return;setProject(p);if(tab==='build'||tab==='overview')fillProject(p);
    if(did){const d=await api(`projects/${pid}/deployments/${did}`);if(alive)setDeploy(d);}
    else if(tab==='overview'||tab==='deployments'){const [history,m]=await Promise.all([api(`projects/${pid}/deployments?offset=${offset}`),api(`projects/${pid}/metrics`)]);if(!alive)return;setDeployments(history.deployments);setMetrics(m);}
    else if(tab==='build'){const [names,gh]=await Promise.all([api(`projects/${pid}/secrets`),api(`projects/${pid}/github`)]);if(!alive)return;setSecrets(names);setGithub(gh);}
    else if(tab==='domains'){const d=await api(`projects/${pid}/domains`);if(!alive)return;setDomains(d.domains);setDomainsInfo(d);}
    else if(tab==='process'){const [status,logs]=await Promise.all([api(`projects/${pid}/process`),fetch(`${base}/api/projects/${pid}/process/logs`).then(r=>r.text())]);if(!alive)return;setProcess(status);setProcessLogs(logs);}
    else if(tab==='access'){const m=await api(`projects/${pid}/members`);if(!alive)return;setMembers(m);if(globalAdmin){const t=await api('team');if(alive)setTeam(t.users);}}
    else if(tab==='audit'){const list=await api(`audit?project=${pid}`);if(alive)setAudit(list);}
   }else if(routeParts[0]==='settings'){const x=await api('settings');if(!alive)return;setSettings(x);setFields(f=>({...f,...x.settings,caddyPort:String(x.settings.caddyPort),retain:String(x.settings.retain),concurrency:String(x.settings.concurrency)}));}
   else if(routeParts[0]==='team'){const t=await api('team');if(!alive)return;setTeam(t.users);setTeamGuidance(t.invitations);const selected=t.users.find((u:any)=>u.id===routeParts[1]);if(selected)setFields(f=>({...f,globalRole:selected.role==='owner'?'viewer':selected.role}));}
   else if(routeParts[0]==='audit'){const list=await api('audit');if(alive)setAudit(list);}
  }catch(e:any){if(alive)setError(e.message);}finally{if(alive)setBusy('');}
 })();return()=>{alive=false;};},[user,route,epoch,offset,api]);
 useEffect(()=>{if(!did||!pid||!user)return;setBuildLogs('');setStreamStatus('Connecting to live log…');const events=new EventSource(`${base}/api/projects/${pid}/deployments/${did}/events`);events.onmessage=e=>{try{const x=JSON.parse(e.data);setDeploy(x.deploy);if(x.text)setBuildLogs(s=>(s+x.text).slice(-2*1024*1024));const running=['queued','preparing','uploading','building','publishing'].includes(x.deploy.state);setStreamStatus(running?'Live · updates every second':'Log complete');if(!running)events.close();}catch{setStreamStatus('Log stream needs a refresh');}};events.onerror=()=>setStreamStatus('Reconnecting; check the application if this persists');return()=>events.close();},[did,pid,user]);
 useEffect(()=>{if(routeParts[0]!=='settings'||!user||!globalAdmin)return;const timer=setInterval(()=>api('settings').then(setSettings).catch((e:any)=>setError(e.message)),4000);return()=>clearInterval(timer);},[route,user,globalAdmin,api]);
 const setField=(name:string,value:any)=>setFields(f=>({...f,[name]:value}));
 const readProject=()=>({name:String(fields.name||''),slug:String(fields.slug||''),repository:String(fields.repository||''),branch:String(fields.branch||'main'),adapter:fields.adapter,nodeMajor:Number(fields.nodeMajor),install:parseArgs(fields.install),build:parseArgs(fields.build),start:parseArgs(fields.start),output:fields.output,baseDirectory:fields.baseDirectory||'.',healthPath:fields.healthPath,timeout:Number(fields.timeout),memoryMB:Number(fields.memoryMB),diskMB:Number(fields.diskMB),previews:!!fields.previews,trusted:!!fields.trusted});
 const navigateDeploy=(d:any)=>{setDeploy(d);changeRoute(`project/${pid||d.projectId}/deploy/${d.id}`);};
 const action=async(name:string)=>{
  if(name==='theme'){const theme=document.documentElement.dataset.theme==='dark'?'light':'dark';document.documentElement.dataset.theme=theme;localStorage.setItem('myhosting-theme',theme);return;}
  if(name==='notifications'){setNotifications(x=>!x);return;}
  if(name==='home'){changeRoute('projects');return;}
  if(name==='createProject'){setFields({...defaults});setStep(0);changeRoute('new');return;}
  if(name==='createBack'){setStep(x=>Math.max(0,x-1));return;}
  if(name==='createNext'){if(step===0&&(!fields.name||!/^[a-z](?:[a-z0-9-]{0,46}[a-z0-9])$/.test(fields.slug))){setError('Enter a name and a lowercase site name (2–48 letters, numbers or hyphens).');return;}if(step===1){try{readProject();}catch(e:any){setError(e.message);return;}}setError('');setStep(x=>Math.min(2,x+1));return;}
  if(name==='triggerDialog'){setShowTrigger(true);setFields(f=>({...f,deployBranch:project?.branch||'main',deployCommit:'',deployEnvironment:'production',clearCache:false}));return;}
  if(name==='closeTrigger'){setShowTrigger(false);return;}
  if(name==='oidcLogin'){sessionStorage.removeItem('myhosting-auth-tried');location.assign(`${base}/auth/login`);return;}
  setError('');setNotice('');setBusy('Working on your request…');
  try{
   if(name==='bootstrap'){const response=await fetch(`${base}/auth/bootstrap`,{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({token:setupToken.current,email:fields.setupEmail,password:fields.setupPassword,confirm:fields.setupConfirm})});const result=await response.json();if(!response.ok)throw new Error(result.error);setupToken.current='';location.hash='projects';location.reload();return;}
   if(name==='localLogin'){const r=await fetch(`${base}/auth/local`,{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({email:fields.loginEmail,password:fields.loginPassword})});if(!r.ok){const b=await r.json();throw new Error(b.error);}location.reload();return;}
   if(name==='saveNewProject'){const p=await api('projects','POST',readProject());setProject(p);changeRoute(`project/${p.id}/overview`);return;}
   if(name==='saveBuild'){await api(`projects/${pid}`,'PUT',readProject());setNotice('Build settings saved. New builds use this configuration.');refresh();return;}
   if(name==='triggerDeploy'){const d=await api(`projects/${pid}/deployments`,'POST',{branch:fields.deployBranch,commit:fields.deployCommit.trim(),environment:fields.deployEnvironment,clearCache:!!fields.clearCache});setShowTrigger(false);navigateDeploy(d);return;}
   if(name==='upload'){const f=await filePick();if(!f)return;if(f.size>64*1024*1024)throw new Error('Source ZIPs are limited to 64 MiB. Do not include node_modules.');setBusy('Uploading source ZIP; keep this page open…');const d=await api(`projects/${pid}/deployments?environment=${fields.uploadEnvironment||'production'}`,'POST',f);navigateDeploy(d);return;}
   const deployActions:Record<string,string>={cancelDeploy:'cancel',retryDeploy:'retry',rebuildDeploy:'rebuild',republishDeploy:'republish',rollbackDeploy:'rollback',promoteDeploy:'promote'};
   if(deployActions[name]){if(['rollback','promote','republish'].includes(deployActions[name])&&!window.confirm('This will switch production traffic after the selected artifact passes its health check. Continue?'))return;const d=await api(`projects/${pid}/deployments/${did}/${deployActions[name]}`,'POST',{});if(d.id)navigateDeploy(d);else setNotice('Cancellation requested. The log will show the final outcome.');return;}
   if(name==='previousPage'){setOffset(x=>Math.max(0,x-30));return;}
   if(name==='nextPage'){setOffset(x=>x+30);return;}
   if(name==='detectFramework'){const x=await api(`projects/${pid}/github/detect`);setFields(f=>({...f,adapter:x.adapter,install:JSON.stringify(x.install),build:JSON.stringify(x.build),start:JSON.stringify(x.start),output:x.output}));setNotice(x.message+' Review the suggested values, then save.');return;}
   if(name==='connectGitHub'){const gh=await api(`projects/${pid}/github`,'POST',{token:fields.githubToken||''});setWebhookSecret(gh.webhookSecret||'');setGithub(gh);setField('githubToken','');setNotice('GitHub access verified. Add the signed push webhook in repository Settings to enable automatic deploys.');return;}
   if(name==='makeCanonical'||name==='clearCanonical'){await api(`projects/${pid}/domains/${selectedDomain.host}/canonical`,'POST',{enabled:name==='makeCanonical'});setNotice('Canonical routing updated and validated.');refresh();return;}
   if(name==='saveSecret'||name==='deleteSecret'){await api(`projects/${pid}/secrets`,name==='saveSecret'?'PUT':'DELETE',{name:fields.secretName,environment:fields.secretEnvironment,value:name==='saveSecret'?fields.secretValue||'':''});setField('secretValue','');setSecrets(await api(`projects/${pid}/secrets`));setNotice('Protected environment updated. Existing deployments keep their saved environment.');return;}
   if(name==='addDomain'){const d=await api(`projects/${pid}/domains`,'POST',{host:fields.domain});changeRoute(`project/${pid}/domains/${d.host}`);return;}
   if(name==='verifyDomain'||name==='removeDomain'){const host=routeParts[3];if(!host)throw new Error('Select a domain first.');if(name==='removeDomain'&&!window.confirm(`Remove the ${host} routing binding? DNS records are not deleted.`))return;const suffix=name==='verifyDomain'?'/verify':'';const d=await api(`projects/${pid}/domains/${host}${suffix}`,name==='verifyDomain'?'POST':'DELETE',{});if(name==='verifyDomain'){setNotice(d.error||'Domain verification finished.');refresh();}else changeRoute(`project/${pid}/domains`);return;}
   if(['processStart','processStop','processRestart'].includes(name)){await api(`projects/${pid}/process`,'POST',{action:name.replace('process','').toLowerCase()});refresh();return;}
   if(name==='refreshProcessLogs'){const response=await fetch(`${base}/api/projects/${pid}/process/logs`);if(!response.ok)throw new Error('Process log unavailable; refresh and check access.');setProcessLogs(await response.text());return;}
   if(name==='saveMember'||name==='removeMember'){await api(`projects/${pid}/members`,name==='saveMember'?'PUT':'DELETE',{userId:fields.memberId,role:fields.memberRole});refresh();return;}
   if(name==='saveGlobalRole'){await api(`team/${routeParts[1]}`,'PUT',{role:fields.globalRole});refresh();return;}
   if(name==='provisionRuntime'){await api(`runtimes/${fields.runtime}`,'POST',{});setNotice('Provisioning started. Runtime cards update automatically; it may take a few minutes.');return;}
   if(name==='repairRuntime'){await api(`runtimes/${fields.runtime}?repair=1`,'POST',{});setNotice('Runtime repair started. Affected apps pause and restart after verification; follow the status below.');return;}
   if(name==='importRuntime'){const f=await filePick();if(!f)return;await api(`runtimes/${fields.runtime}`,'POST',f);setNotice('Archive uploaded. Its pinned checksum is checked before installation.');return;}
   if(name==='saveSettings'){await api('settings','PUT',{caddyPort:Number(fields.caddyPort),public:!!fields.public,transport:String(fields.transport||'direct'),addressStyle:String(fields.addressStyle||'nested'),previewDomain:String(fields.previewDomain||''),retain:Number(fields.retain),concurrency:1});setNotice('Routing settings saved and validated.');refresh();return;}
   if(name==='cleanupStorage'){if(!window.confirm('Delete deployment files beyond the saved retention target? Live releases, active previews, app data, audit history and backups are kept.'))return;const result=await api('storage/cleanup','POST',{});setNotice(`Storage cleanup finished: ${bytes(result.bytes)} removed; ${result.expired} old releases expired.`);refresh();return;}
   if(name==='exportBackup'){setBusy('Pausing Node apps and exporting the encrypted backup…');const blob=await api('backups','POST',{password:fields.backupPassword||''},true);setField('backupPassword','');downloadBlob(blob,`myhosting-${new Date().toISOString().slice(0,10)}.mhbackup`);setNotice('Encrypted backup downloaded. Keep its password separately. Node apps have been resumed.');return;}
   throw new Error('This action is unavailable in the current view.');
  }catch(e:any){setError(e.message||'The request did not finish. Please retry.');}finally{setBusy('');}
 };
 const listDeploy=(d:any)=>({...d,href:deployHref(d.projectId,d.id),tone:tone(d.state),title:d.message||`${d.action==='build'?'Deploy':d.action} · ${d.commit?d.commit.slice(0,8):'Uploaded ZIP'}`,summary:`${d.environment} · ${d.branch||'ZIP'} · ${d.author||'Local source'} · ${d.id.slice(0,8)}`,when:formatDate(d.created)});
 const selectedDomain=domains.find(d=>d.host===routeParts[3]),selectedUser=team.find(u=>u.id===routeParts[1]);
 const q=String(fields.search||'').toLowerCase();
 const projectCards=projects.filter(p=>`${p.name} ${p.slug} ${p.repository}`.toLowerCase().includes(q)).map(p=>({...p,href:`#project/${p.id}/overview`,icon:p.adapter==='static'?'Globe':'Server',status:p.lastState||(p.live?'published':'not deployed'),tone:tone(p.lastState||(p.live?'published':'')),description:p.lastMessage||(p.live?'Production release available':'Ready for its first deployment'),repository:p.repository||'ZIP source · no repository connected'}));
 const filteredDeploys=deployments.filter(d=>fields.deployFilter==='all'||d.state===fields.deployFilter).map(listDeploy);
 const production=deployments.find(d=>d.id===project?.live);
 const running=deploy&&['queued','preparing','uploading','building','publishing'].includes(deploy.state),published=deploy?.state==='published';
 const seconds=deploy?.started?Math.max(0,(new Date(deploy.finished||Date.now()).getTime()-new Date(deploy.started).getTime())/1000):0;
 const data:any={logo,user:user||{},initials:(user?.name||user?.email||'MH').split(/[\s@]+/).slice(0,2).map((x:string)=>x[0]).join('').toUpperCase(),workspaceURL:'/',workspaceLinked:boot.oidc&&base==='/hosting',error,notice,busy,version:boot.version||'0.1.0-preview.1',oidc:boot.oidc,bootstrapNeeded:!boot.configured,
  nav:[['projects','Projects','Layers'],['team','Team & access','UsersRound'],['settings','Settings & health','Settings2'],['audit','Audit trail','ScrollText']].filter(x=>globalAdmin||x[0]==='projects').map(([id,title,icon])=>({href:`#${id}`,title,icon,variant:(id===routeParts[0]||id==='projects'&&pid)?'nav-active':'nav'})),
  canAdmin:pid?canAdmin:globalAdmin,canDeploy,isOwner:user?.role==='owner',projects:projectCards,projectCount:projects.length,liveCount:projects.filter(p=>p.live).length,emptyProjects:!projectCards.length,
  createSource:step===0,createBuild:step===1,createReview:step===2,sourceTone:step===0?'info':'success',buildTone:step===1?'info':step>1?'success':'muted',reviewTone:step===2?'info':'muted',suggestedSiteURL,createSummary:`${fields.name} · ${suggestedSiteURL} · ${fields.adapter} · ${fields.repository||'ZIP uploads'} · ${fields.branch}`,
  adapterOptions:adapters,nodeOptions:options(['24','22']),environmentOptions:options(['production','preview']),projectRoleOptions:options(['viewer','developer','admin']),globalRoleOptions:options(['viewer','admin','disabled']),runtimeOptions:[{value:'caddy',label:'Caddy · static/Node routing'},{value:'node24',label:'Node 24 · shared runtime'},{value:'node22',label:'Node 22 · shared runtime'}],concurrencyOptions:options(['1']),addressStyleOptions:[{value:'flat',label:'Flat · my-site-sites.example.com (free tunnel SSL)'},{value:'nested',label:'Nested · my-site.sites.example.com'}],transportOptions:[{value:'direct',label:'Direct HTTPS · Caddy certificates'},{value:'tunnel',label:'HTTPS tunnel · certificates at the edge'}],
  canUseCanonical:!!selectedDomain?.verified&&selectedDomain?.https==='certificate-valid'&&!selectedDomain?.canonical,project:project||{},projectTabs:[['overview','Overview'],['deployments','Deployments'],['domains','Domains & HTTPS'],['build','Build settings'],['process','Logs & processes'],['access','Access'],['audit','Audit']].map(([id,title])=>({href:`#project/${pid}/${id}`,title,variant:id===tab?'tab-active':'tab'})),
  tabOverview:tab==='overview',tabDeployments:tab==='deployments',tabBuild:tab==='build',tabDomains:tab==='domains',tabProcess:tab==='process',tabAccess:tab==='access',tabAudit:tab==='audit'||routeParts[0]==='audit',
  deploymentsHref:`#project/${pid}/deployments`,productionStatus:project?.live?'Published':'Not deployed',productionTone:project?.live?'success':'muted',productionMessage:production?.message||(project?.live?'A release is serving this project. Open deployment history for its source and logs.':'Your first successful deployment will appear here.'),productionURL:production?.url||(project?.live?`${location.protocol==='https:'?'http:':'http:'}//${project.slug}.localhost:${settings.settings?.caddyPort||9080}`:''),productionCommit:production?`${production.commit||'ZIP source'} · ${formatDate(production.finished)}`:'',
  metrics,metricsLabel:'Observed traffic',recentDeploys:deployments.slice(0,4).map(listDeploy),noDeploys:deployments.length===0,deployments:filteredDeploys,noFilteredDeploys:filteredDeploys.length===0,deployFilters:options(['all','queued','preparing','uploading','building','publishing','published','failed','cancelled','expired']),firstPage:offset===0,lastPage:deployments.length<30,pageLabel:`Showing ${offset+1}–${offset+deployments.length}`,
  github:{...github,message:github.message||(github.privateCredentialSaved?'A protected repository credential is saved.':'Public repositories work without a token; private repositories need fine-grained Contents:read access.')},webhookSecret,secrets,
  domains:domains.map(d=>({...d,href:`#project/${pid}/domains/${d.host}`,tone:d.verified?'success':'warning',status:d.https==='certificate-valid'?'HTTPS certificate valid':d.verified?'DNS verified · HTTPS pending':'Needs DNS verification'})),selectedDomain,dnsRecord:selectedDomain?`_one-workspace.${selectedDomain.host}`:'',domainGuidance:domainsInfo.guidance||'',localURL:domainsInfo.localURL||'',
  process,processLogs,processSummary:`Port ${process.port||'—'} · PID ${process.pid||'—'} · Restarts ${process.restarts||0}`,
  members,memberOptions:[{value:'',label:'Select a signed-in member'},...(globalAdmin?team:members).map(u=>({value:u.id,label:`${u.name} · ${u.email||u.id.slice(0,8)}`}))],audit:audit.map(e=>({...e,at:formatDate(e.at),summary:JSON.stringify(e.detail)})),showTrigger,
  deploy:deploy||{config:{}},deployTitle:deploy?.message||`Deployment ${did?.slice(0,8)||''}`,deploySummary:`${deploy?.action||''} · ${deploy?.author||'Uploaded source'} · ${deploy?.id||''}`,deployTone:tone(deploy?.state||''),shortCommit:deploy?.commit?.slice(0,8)||'ZIP source',duration:seconds?`${Math.floor(seconds/60)}m ${Math.round(seconds%60)}s`:'Queued',outputSize:bytes(deploy?.bytes||0),nodeVersion:deploy?.config?.adapter==='static'?'Static files':`Node ${deploy?.config?.nodeMajor||24}`,canCancel:canDeploy&&running,canRetry:canDeploy&&['failed','cancelled'].includes(deploy?.state),canRebuild:canDeploy&&deploy&&!running,canRepublish:canDeploy&&published,canRollback:canAdmin&&published&&deploy?.environment==='production',canPromote:canDeploy&&published&&deploy?.environment==='preview',downloadLogsURL:`${base}/api/projects/${pid}/deployments/${did}/logs`,streamStatus,streamTone:running?'info':'muted',buildLogs:buildLogs||(deploy?.state==='expired'?'This deployment was removed by retention. Its metadata and audit history remain.':'Waiting for the first log output…'),configJSON:JSON.stringify(deploy?.config||{},null,2),sourceProof:`Source SHA-256: ${deploy?.sourceHash||'Pending download'}`,artifactProof:`Retained artifact: ${deploy?.artifact||'Pending build'} · parent: ${deploy?.parent||'none'}`,
  settings,runtimes:(settings.runtimes||[]).map((r:any)=>({...r,name:r.id==='caddy'?'Caddy':r.id==='node24'?'Node 24':'Node 22',tone:tone(r.state),description:r.id==='caddy'?'Native HTTP(S) routing and certificates':'Installed only for projects requiring this major version'})),runtimeStatus:(settings.runtimes||[]).find((r:any)=>r.id===fields.runtime)?.state||'Not provisioned',caddySummary:JSON.stringify(settings.caddy||{}),releasesURL:'https://github.com/oneworkspacestudio/my-hosting/releases',
  team:team.map(u=>({...u,href:`#team/${u.id}`,tone:u.role==='disabled'?'error':'muted',identity:`${u.issuer} · ${u.subject}`})),teamGuidance,selectedUser,
 };
 const componentProps:any={data,onAction:(name:string)=>{void action(name);}};
 let screen:React.ReactNode;
 if(loading)screen=React.createElement('main',{className:'card card--login',role:'status'},'Connecting to My Hosting…');
 else if(!user)screen=React.createElement(setupToken.current&&boot.setup?Setup:Login,componentProps);
 else{let Component:any=Projects;if(routeParts[0]==='new')Component=Create;else if(pid)Component=did?Deploy:Project;else if(routeParts[0]==='settings')Component=Settings;else if(routeParts[0]==='team')Component=Team;else if(routeParts[0]==='audit')Component=Audit;const outlet=React.createElement(Component,componentProps);screen=React.createElement(Fields.Provider,{value:{fields,setField,disabled:!!busy,outlet}},React.createElement(Shell,componentProps));}
 const drawer=notifications&&user?React.createElement('aside',{className:'notice-drawer','aria-label':'Recent deployment notifications'},React.createElement('h2',{},'Deployment activity'),React.createElement('button',{className:'button button--secondary',onClick:()=>setNotifications(false)},'Close'),...(deployments.length?deployments.slice(0,10).map(d=>React.createElement('a',{key:d.id,href:deployHref(d.projectId,d.id),onClick:()=>setNotifications(false)},`${d.state} · ${d.message||d.id.slice(0,8)} · ${formatDate(d.created)}`)):[React.createElement('span',{key:'empty',className:'text text--muted'},'Open a project to see its recent deployment activity.')])):null;
 return React.createElement(Fields.Provider,{value:{fields,setField,disabled:!!busy}},screen,drawer);
}

createRoot(document.getElementById('root')!).render(React.createElement(Application));
