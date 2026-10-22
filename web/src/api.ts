import {t} from './i18n'
import {bindOfflineUser,cachedUser,clearOffline,offlineRead,queueWrite,hasQueued,rememberRead,cachedImage,networkOnline} from './offline'
export interface User { id:string; name:string; space_id:string }
export interface Photo { id:string; filename:string; title:string; note:string; place:string; taken_at:string; latitude:number|null; longitude:number|null; owner_id:string; created_at:string }
export class ApiError extends Error { constructor(message:string,public status:number){super(message);this.name='ApiError'} }
export async function api<T>(path:string, options:RequestInit = {}):Promise<T> {
 const method=options.method||'GET'
 if(method==='PUT'&&await hasQueued(path))return await queueWrite(path,options) as T
 let response:Response
 try{
  if(!navigator.onLine)throw new TypeError('Offline')
  if(localStorage.getItem('footprint-logout-pending')){const logout=await fetch('/api/auth/logout',{method:'POST'});if(!logout.ok&&logout.status!==401)throw new TypeError('Logout pending');localStorage.removeItem('footprint-logout-pending')}
  response=await fetch(`/api${path}`, { ...options, headers: options.body instanceof FormData ? options.headers : {'Content-Type':'application/json',...options.headers} })
 }catch(error){
  if((error as Error).name==='AbortError')throw error
  networkOnline.value=false
  if(path==='/auth/logout'){localStorage.setItem('footprint-logout-pending','true');await clearOffline();await bindOfflineUser(null);return undefined as T}
  if(path==='/auth/me'){const user=await cachedUser();if(user)await bindOfflineUser(user)}
  if(method==='GET')return await offlineRead(path) as T
  return await queueWrite(path,options) as T
 }
 networkOnline.value=true
 if(!response.ok){const error=await response.json().catch(()=>({error:t('服务器暂时无法响应')}));throw new ApiError(error.error||t('请求失败'),response.status)}
 const value=response.status===204?undefined:await response.json()
 if(path==='/auth/me'||path==='/auth/login'||path==='/auth/register'||path==='/auth/pair'||path==='/auth/unbind')await bindOfflineUser(value)
 if(path==='/auth/logout'){await clearOffline();await bindOfflineUser(null)}
 return (method==='GET'?await rememberRead(path,value):value) as T
}

export const originalMedia=(id:string)=>`/api/photos/${encodeURIComponent(id)}/file`
export const media = (id:string) => !networkOnline.value&&cachedImage(id)?cachedImage(id)!:`/api/photos/${encodeURIComponent(id)}/file`

export type MemoryMode = 'travel' | 'food' | 'love'
export interface Memory {
 id:string; title:string; description:string; mode:MemoryMode; date:string; place:string;
 province:string; city:string; latitude:number|null; longitude:number|null; tags:string[];
 rating:number; price:number; cover_id:string; owner_id:string; created_at:string; updated_at:string; photos:Photo[]
}
export const modes = { travel:{label:t('旅行探索'),color:'#38bdf8'},food:{label:t('美食寻味'),color:'#fbbf24'},love:{label:t('专属回忆'),color:'#f48fb1'} }
export const thumbnail=(id:string)=>!networkOnline.value&&cachedImage(id)?cachedImage(id)!:`/api/photos/${encodeURIComponent(id)}/thumb`
