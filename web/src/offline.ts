import {t} from './i18n'
import { ref } from 'vue'
import type {Memory,User,Photo} from './api'
export interface OfflineWrite {id:string;path:string;body:Record<string,unknown>;base:string;status:'pending'|'conflict'|'error';message:string;created_at:string}
interface Feature {data:unknown;revision:string}
interface Snapshot {user:User;memories:Memory[];members:User[];features:Record<string,Feature>;revision:number;system:unknown;updated_at:string}
interface State {snapshot:Snapshot|null;queue:OfflineWrite[]}
const keys=['anniversaries','love_notes','date_plans','wishes','love_capsules','packing_list','ingredients','travel_plans','expenses','food_choices','space_settings']
export const offlineEnabled=ref(false),pendingWrites=ref<OfflineWrite[]>([]),offlineMessage=ref(''),networkOnline=ref(navigator.onLine),syncing=ref(false),offlineSavedAt=ref(''),cachedPictures=ref(0)
let identity:User|null=null,dbPromise:Promise<IDBDatabase>|undefined
const blobUrls=new Map<string,string>()
const channel=typeof BroadcastChannel!=='undefined'?new BroadcastChannel('footprint-offline'):null
channel?.addEventListener('message',()=>{void get<State>('state').then(state=>{if(state)publish(state);else{offlineEnabled.value=false;pendingWrites.value=[]}}).catch(()=>{})})
function database(){return dbPromise??=new Promise<IDBDatabase>((resolve,reject)=>{const request=indexedDB.open('footprint-offline-v1',1);request.onupgradeneeded=()=>request.result.createObjectStore('records');request.onsuccess=()=>resolve(request.result);request.onerror=()=>{dbPromise=undefined;reject(request.error)}})}
async function get<T>(key:string):Promise<T|undefined>{const db=await database();return new Promise((resolve,reject)=>{const r=db.transaction('records').objectStore('records').get(key);r.onsuccess=()=>resolve(r.result);r.onerror=()=>reject(r.error)})}
async function put(key:string,value:unknown){const db=await database();return new Promise<void>((resolve,reject)=>{const tx=db.transaction('records','readwrite');tx.objectStore('records').put(value,key);tx.oncomplete=()=>resolve();tx.onabort=()=>reject(tx.error);tx.onerror=()=>reject(tx.error)})}
async function update(change:(state:State)=>void){const db=await database();return new Promise<State>((resolve,reject)=>{const tx=db.transaction('records','readwrite'),store=tx.objectStore('records'),request=store.get('state');let state:State;request.onsuccess=()=>{state=request.result||{snapshot:null,queue:[]};try{change(state);store.put(state,'state')}catch(e){tx.abort();reject(e)}};tx.oncomplete=()=>{publish(state);channel?.postMessage('changed');resolve(state)};tx.onabort=()=>reject(tx.error||new Error(t('无法保存到本机，请检查浏览器存储空间')));tx.onerror=()=>reject(tx.error)})}
function same(user:User|null|undefined){return !!identity&&user?.id===identity.id&&user?.space_id===identity.space_id}
function publish(state:State){const enabled=same(state.snapshot?.user);offlineEnabled.value=enabled;pendingWrites.value=enabled?state.queue:[];offlineSavedAt.value=enabled?state.snapshot!.updated_at:''}
export async function bindOfflineUser(user:User|null){identity=user;if(!user){offlineEnabled.value=false;pendingWrites.value=[];return}try{const state=await get<State>('state');if(state?.snapshot&&state.snapshot.user.id!==user.id){await clearOffline();identity=user}else if(state){publish(state);if(!same(state.snapshot?.user)&&state.queue.length)offlineMessage.value=t('空间发生变化，旧空间的离线修改不会自动合并。请先导出草稿。');await loadImages()}}catch{offlineMessage.value=t('此浏览器暂时无法保存离线数据')}}
export async function clearOffline(){const db=await database();await new Promise<void>((resolve,reject)=>{const tx=db.transaction('records','readwrite');tx.objectStore('records').clear();tx.oncomplete=()=>resolve();tx.onerror=()=>reject(tx.error)});for(const url of blobUrls.values())URL.revokeObjectURL(url);blobUrls.clear();offlineEnabled.value=false;pendingWrites.value=[];offlineSavedAt.value='';cachedPictures.value=0;offlineMessage.value='';channel?.postMessage('changed')}
async function stateForUser(){try{const state=await get<State>('state');return state&&same(state.snapshot?.user)?state:null}catch{return null}}
export async function cachedUser(){try{const state=await get<State>('state');return state?.snapshot?.user||null}catch{return null}}
export function cachedImage(id:string){return blobUrls.get(id)}
async function loadImages(){for(const url of blobUrls.values())URL.revokeObjectURL(url);blobUrls.clear();if(!offlineEnabled.value)return;const db=await database();await new Promise<void>((resolve,reject)=>{const r=db.transaction('records').objectStore('records').openCursor();r.onsuccess=()=>{const cursor=r.result;if(!cursor){resolve();return}if(String(cursor.key).startsWith('thumb:')&&cursor.value instanceof Blob)blobUrls.set(String(cursor.key).slice(6),URL.createObjectURL(cursor.value));cursor.continue()};r.onerror=()=>reject(r.error)});cachedPictures.value=blobUrls.size}
async function raw(path:string,options:RequestInit={}){const response=await fetch('/api'+path,{...options,credentials:'same-origin',signal:options.signal||AbortSignal.timeout(20000),headers:{'Content-Type':'application/json',...options.headers}});if(!response.ok){const data=await response.json().catch(()=>({}));throw Object.assign(new Error(data.error||t('请求失败')),{status:response.status})}return response.status===204?undefined:response.json()}
export async function enableOffline(){
 if(!identity)throw new Error(t('请先登录'));offlineMessage.value=t('正在准备离线副本…')
 const user=await raw('/auth/me') as User;if(!same(user))throw new Error(t('账号或空间已变化，请重新登录'))
 const revision=await raw('/revision'),library=await raw('/memories'),people=await raw('/library'),system=await raw('/system'),features:Record<string,Feature>={}
 for(const key of keys)features[key]=await raw('/features/'+key)
 if(!same(user))throw new Error(t('账号或空间已变化，请重试'))
 const old=await get<State>('state');if(old?.queue.length)throw new Error(t('请先同步或处理待同步修改，再更新离线副本'))
 const snapshot:Snapshot={user,memories:library.memories,members:people.members,features,revision:revision.revision,system,updated_at:new Date().toISOString()}
 await update(state=>{state.snapshot=snapshot;state.queue=[]})
 // Cache bounded thumbnails, never original photo files or auth responses in SW.
 const photos=(snapshot.memories.flatMap(m=>m.photos) as Photo[]).slice(0,200)
 let bytes=0
 for(const photo of photos){try{const response=await fetch('/api/photos/'+encodeURIComponent(photo.id)+'/thumb',{signal:AbortSignal.timeout(10000)});if(!response.ok)continue;const blob=await response.blob();if(!same(user))break;if(bytes+blob.size>40*1024*1024)break;await put('thumb:'+photo.id,blob);bytes+=blob.size}catch{break}}
 await loadImages();offlineMessage.value=t('离线副本已保存，断网时可编辑回忆和共同清单');try{await navigator.storage?.persist?.()}catch{}
}
function overlay(snapshot:Snapshot,queue:OfflineWrite[]):Snapshot{
 const next=structuredClone(snapshot)
 for(const item of queue){if(item.path.startsWith('/memories/')){const id=item.path.slice(10),index=next.memories.findIndex(m=>m.id===id);if(index>=0){const memory=next.memories[index]!;Object.assign(memory,item.body,{updated_at:'offline:'+item.id});const ids=item.body.photo_ids as string[]|undefined;if(ids)memory.photos=ids.map(id=>memory.photos.find(p=>p.id===id)).filter((p):p is Photo=>!!p)}}else{const key=item.path.slice('/features/'.length);next.features[key]={data:item.body.data,revision:'offline:'+item.id}}}
 return next
}
export async function offlineRead(path:string):Promise<unknown>{
 const state=await stateForUser();if(!state?.snapshot)throw new Error(t('尚未保存离线副本，请联网后在双人空间开启离线使用'))
 const snapshot=overlay(state.snapshot,state.queue),url=new URL(path,'https://offline.local'),p=url.pathname
 if(p==='/auth/me')return snapshot.user
 if(p==='/setup')return {initialized:true}
 if(p==='/revision')return {revision:-state.queue.length-1}
 if(p==='/system')return snapshot.system
 if(p==='/library')return {memories:snapshot.memories,members:snapshot.members}
 if(p.startsWith('/features/'))return snapshot.features[p.slice(10)]||{data:[],revision:''}
 if(p.startsWith('/memories/')){const memory=snapshot.memories.find(m=>m.id===p.slice(10));if(!memory)throw new Error(t('此回忆尚未保存在本机'));return memory}
 if(p==='/memories'){
  const q=url.searchParams,search=(q.get('q')||'').toLowerCase(),items=snapshot.memories.filter(m=>(!q.get('mode')||m.mode===q.get('mode'))&&(!q.get('year')||m.date.startsWith(q.get('year')!))&&(!q.get('place')||m.place===q.get('place')||m.city===q.get('place'))&&(!search||`${m.title} ${m.description} ${m.place} ${m.city} ${m.tags.join(' ')}`.toLowerCase().includes(search)))
  const page=Number(q.get('page')||1),size=Number(q.get('per_page')||30);return {memories:q.has('page')?items.slice((page-1)*size,page*size):items,total:items.length,page,per_page:size}
 }
 throw new Error(t('此操作需要联网后进行'))
}
export async function rememberRead(path:string,value:unknown):Promise<unknown>{
 const state=await stateForUser();if(!state?.snapshot)return value
 const p=new URL(path,'https://offline.local').pathname,data=value as any
 if(!['/library','/memories','/revision','/system'].includes(p)&&!p.startsWith('/memories/')&&!p.startsWith('/features/'))return value
 try{const next=await update(state=>{if(!same(state.snapshot?.user))return;const s=state.snapshot!
  if(p==='/library'){s.members=data.members;s.memories=data.memories.map((m:Memory)=>{const previous=s.memories.find(old=>old.id===m.id);return {...m,photos:m.photos.map(photo=>({...photo,note:previous?.photos.find(old=>old.id===photo.id)?.note||photo.note}))}})}
  else if(p==='/memories'&&!new URL(path,'https://offline.local').searchParams.has('page'))s.memories=data.memories
  else if(p.startsWith('/memories/')){const index=s.memories.findIndex(m=>m.id===data.id);if(index>=0)s.memories[index]=data;else s.memories.push(data)}
  else if(p.startsWith('/features/'))s.features[p.slice(10)]=data
  else if(p==='/revision')s.revision=data.revision
  else if(p==='/system')s.system=data
  s.updated_at=new Date().toISOString()
 });if(next.queue.length&&(p==='/library'||p==='/memories'||p.startsWith('/memories/')||p.startsWith('/features/')))return offlineRead(path)}catch{offlineMessage.value=t('离线副本更新失败，请检查本机剩余空间')}
 return value
}
export async function hasQueued(path:string){const state=await stateForUser();return !!state?.queue.some(item=>item.path===path)}
export async function queueWrite(path:string,options:RequestInit){
 if(options.method!=='PUT'||(!path.startsWith('/memories/')&&!path.startsWith('/features/')))throw new Error(t('离线时可编辑回忆和共同清单；上传、删除及账号操作请联网后进行'))
 const body=JSON.parse(String(options.body||'{}')) as Record<string,unknown>,id=crypto.randomUUID()
 const state=await update(state=>{
  if(!same(state.snapshot?.user))throw new Error(t('请联网后在双人空间开启离线使用'))
  if(path.startsWith('/memories/')){const memory=state.snapshot!.memories.find(m=>m.id===path.slice(10));if(!memory)throw new Error(t('此回忆尚未保存到本机'));const ids=body.photo_ids as string[]|undefined;if(ids&&(ids.length!==memory.photos.length||ids.some(id=>!memory.photos.some(p=>p.id===id))))throw new Error(t('离线时暂不支持移动照片到其他回忆'))}
  const existing=state.queue.find(item=>item.path===path),base=existing?.base??String(path.startsWith('/memories/')?body.updated_at:body.revision)
  if(existing){existing.body=body;existing.status='pending';existing.message=''}else state.queue.push({id,path,body,base,status:'pending',message:'',created_at:new Date().toISOString()})
 })
 const queued=state.queue.find(item=>item.path===path)!
 offlineMessage.value=t('修改已保存在本机，联网后会自动同步')
 return path.startsWith('/features/')?{revision:'offline:'+queued.id}:offlineRead(path)
}
export async function syncOffline(){
 if(syncing.value||!networkOnline.value||!identity)return
 const initial=await stateForUser();if(!initial?.queue.length)return
 // A short renewable IndexedDB lease also protects browsers without Web Locks.
 const lease=crypto.randomUUID(),db=await database()
 const acquired=await new Promise<boolean>((resolve,reject)=>{const tx=db.transaction('records','readwrite'),store=tx.objectStore('records'),r=store.get('lease');let own=false;r.onsuccess=()=>{if(!r.result||r.result.until<Date.now()){store.put({id:lease,until:Date.now()+60000},'lease');own=true}};tx.oncomplete=()=>resolve(own);tx.onerror=()=>reject(tx.error)})
 if(!acquired)return
 syncing.value=true
 try{
  const user=await raw('/auth/me') as User
  if(!same(user)||!same(initial.snapshot?.user)){offlineMessage.value=t('账号或空间已变化，离线修改已暂停，请先导出草稿');return}
  for(const pending of initial.queue){
   const live=await stateForUser(),item=live?.queue.find(q=>q.id===pending.id);if(!item||item.status!=='pending')continue
   await put('lease',{id:lease,until:Date.now()+60000})
   const body={...item.body,[item.path.startsWith('/memories/')?'updated_at':'revision']:item.base}
   try{
    const response=await raw(item.path,{method:'PUT',body:JSON.stringify(body)})
    const fresh=await raw(item.path)
    await update(state=>{const latest=state.queue.find(q=>q.id===item.id);if(!latest)return
     if(item.path.startsWith('/memories/')){const index=state.snapshot!.memories.findIndex(m=>m.id===fresh.id);if(index>=0)state.snapshot!.memories[index]=fresh}else state.snapshot!.features[item.path.slice(10)]=fresh
     if(JSON.stringify(latest.body)===JSON.stringify(item.body))state.queue=state.queue.filter(q=>q.id!==item.id)
     else latest.base=String(item.path.startsWith('/memories/')?fresh.updated_at:response.revision)
    })
   }catch(error){const e=error as Error&{status?:number};if(!e.status){networkOnline.value=false;break}
    if(e.status===401||e.status===403){offlineMessage.value=t('登录或空间权限已变化，离线修改已暂停');break}
    await update(state=>{const latest=state.queue.find(q=>q.id===item.id);if(latest){latest.status=e.status===409?'conflict':'error';latest.message=e.message}})
   }
  }
  if(!pendingWrites.value.length)offlineMessage.value=t('离线修改已全部同步')
 }finally{syncing.value=false;await put('lease',{id:lease,until:0})}
}
export async function resolveWrite(id:string,action:'server'|'local'){
 const state=await stateForUser(),item=state?.queue.find(q=>q.id===id);if(!item)return
 const user=await raw('/auth/me') as User;if(!same(user))throw new Error(t('账号或空间已变化，请重新登录'))
 const fresh=await raw(item.path)
 await update(state=>{const queued=state.queue.find(q=>q.id===id);if(!queued)return
  if(item.path.startsWith('/memories/')){const index=state.snapshot!.memories.findIndex(m=>m.id===fresh.id);if(index>=0)state.snapshot!.memories[index]=fresh}else state.snapshot!.features[item.path.slice(10)]=fresh
  if(action==='server')state.queue=state.queue.filter(q=>q.id!==id)
  else{queued.base=String(item.path.startsWith('/memories/')?fresh.updated_at:fresh.revision);queued.status='pending';queued.message=''}
 });if(action==='local')await syncOffline()
}
export async function exportDrafts(){const state=await get<State>('state');if(!identity||!state?.snapshot||state.snapshot.user.id!==identity.id)throw new Error(t('请登录原账号后导出草稿'));const url=URL.createObjectURL(new Blob([JSON.stringify({user:state.snapshot.user,queue:state.queue},null,2)],{type:'application/json'}));const link=document.createElement('a');link.href=url;link.download='footprint-offline-drafts.json';link.click();setTimeout(()=>URL.revokeObjectURL(url),1000)}
window.addEventListener('online',()=>{networkOnline.value=true;void syncOffline()});window.addEventListener('offline',()=>networkOnline.value=false)
