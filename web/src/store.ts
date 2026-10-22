import {networkOnline,syncOffline,pendingWrites} from './offline'
import { defineStore } from 'pinia'
import { ref } from 'vue'
import { api,ApiError,type Photo,type User,type Memory } from './api'
export const useAlbum=defineStore('album',()=>{
 const user=ref<User|null>(null),photos=ref<Photo[]>([]),memories=ref<Memory[]>([]),members=ref<User[]>([]),ready=ref(false),spaceSettings=ref<Record<string,unknown>>({}),online=networkOnline,revision=ref(-1)
 let refreshing:Promise<void>|null=null,lastRevision=-1
 function refresh():Promise<void>{
  if(refreshing)return refreshing
  refreshing=(async()=>{
   // Read revision first: a concurrent write during loading is detected by the next poll.
   const before=await api<{revision:number}>('/revision')
   const [library,settings]=await Promise.all([api<{memories:Memory[];members:User[]}>('/library'),api<{data:Record<string,unknown>}>('/features/space_settings')])
   photos.value=library.memories.flatMap(m=>m.photos).sort((a,b)=>b.taken_at.localeCompare(a.taken_at)||b.created_at.localeCompare(a.created_at));members.value=library.members;memories.value=library.memories;spaceSettings.value=Array.isArray(settings.data)?{}:settings.data;lastRevision=before.revision;revision.value=before.revision
  })().finally(()=>refreshing=null)
  return refreshing
 }
 async function init(){try{user.value=await api<User>('/auth/me');await refresh()}catch{user.value=null}finally{ready.value=true}}
 function clear(){user.value=null;photos.value=[];memories.value=[];members.value=[];spaceSettings.value={};lastRevision=-1;revision.value=-1}
 async function logout(){try{await api('/auth/logout',{method:'POST'})}finally{clear()}}
 function startSync(){let active=true,checking=false
  const check=async()=>{if(!active||checking||!user.value||document.hidden)return;checking=true;try{const current=await api<{revision:number}>('/revision');if(networkOnline.value&&pendingWrites.value.some(item=>item.status==='pending')){await syncOffline();await refresh()}else if(active&&current.revision!==lastRevision)await refresh()}catch(e){online.value=false;if(e instanceof ApiError&&e.status===401)clear()}finally{checking=false}}
  const timer=setInterval(check,10000);window.addEventListener('focus',check);document.addEventListener('visibilitychange',check)
  return ()=>{active=false;clearInterval(timer);window.removeEventListener('focus',check);document.removeEventListener('visibilitychange',check)}
 }
 return {user,photos,memories,members,ready,spaceSettings,online,revision,refresh,init,logout,startSync}
})
