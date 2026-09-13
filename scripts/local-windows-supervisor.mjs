// Project-local Windows process supervision. No model credentials are loaded.
import fs from 'node:fs';
import path from 'node:path';
import {fileURLToPath} from 'node:url';
import {spawn} from 'node:child_process';
const root=path.resolve(path.dirname(fileURLToPath(import.meta.url)),'..');
const dir=path.join(root,'data/windows');
const secrets=JSON.parse(fs.readFileSync(path.join(dir,'secrets.json'),'utf8').replace(/^\uFEFF/,''));
const env={...process.env,APP_ENV:'development',PORT:'18000',FRONTEND_PORT:'13000',DATABASE_URL:`postgres://loretide_local_admin:${secrets.password}@127.0.0.1:15332/loretide_dev?sslmode=disable`,JWT_SECRET:secrets.jwt,FRONTEND_ORIGIN:'http://localhost:13000',CORS_ALLOWED_ORIGINS:'http://localhost:13000',REMOTE_API_URL:'http://127.0.0.1:18000',LORETIDE_EXECUTION_POLICY:'disabled',LORETIDE_DIAGNOSTICS_TEST:'1',LORETIDE_BUILD:'windows-local-migration',LOCAL_UPLOAD_DIR:path.join(root,'server/data/uploads'),NEXT_TELEMETRY_DISABLED:'1',NODE_OPTIONS:'--max-old-space-size=4096'};
let stopping=false;
const children=new Map();
const stopFile=path.join(dir,'stop');
if(fs.existsSync(stopFile))fs.unlinkSync(stopFile);
fs.writeFileSync(path.join(dir,'supervisor.pid'),String(process.pid));
function launch(name,exe,args,cwd){
  if(stopping)return;
  const log=fs.openSync(path.join(dir,`${name}.log`),'a');
  const child=spawn(exe,args,{cwd,env,windowsHide:true,stdio:['ignore',log,log]});
  fs.closeSync(log);children.set(name,child);
  fs.writeFileSync(path.join(dir,`${name}.pid`),String(child.pid));
  child.on('error',e=>console.error(name,e.message));
  child.on('exit',code=>{children.delete(name);console.log(name,'exited',code);if(!stopping)setTimeout(()=>launch(name,exe,args,cwd),3000);});
}
launch('api',path.join(dir,'api.exe'),[],path.join(root,'server'));
launch('web',process.execPath,[path.join(root,'apps/web/node_modules/next/dist/bin/next'),'dev','--webpack','--hostname','127.0.0.1','--port','13000'],path.join(root,'apps/web'));
setInterval(()=>{if(fs.existsSync(stopFile)&&!stopping){stopping=true;for(const c of children.values())spawn('taskkill',['/PID',String(c.pid),'/T','/F'],{windowsHide:true});setTimeout(()=>process.exit(0),3000);}},1000);
