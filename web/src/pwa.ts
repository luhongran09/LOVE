import { ref } from 'vue'
interface InstallPrompt extends Event { prompt():Promise<void>;userChoice:Promise<{outcome:string}> }
export const installPrompt=ref<InstallPrompt|null>(null),updateWorker=ref<ServiceWorker|null>(null),pwaAvailable=ref(false)
export async function startPwa(){
 window.addEventListener('beforeinstallprompt',event=>{event.preventDefault();installPrompt.value=event as InstallPrompt})
 if(!('serviceWorker' in navigator)||!window.isSecureContext)return
 try{
  const registration=await navigator.serviceWorker.register('/sw.js',{scope:'/',updateViaCache:'none'});pwaAvailable.value=true
  const update=()=>{if(registration.waiting&&navigator.serviceWorker.controller)updateWorker.value=registration.waiting}
  update();registration.addEventListener('updatefound',()=>registration.installing?.addEventListener('statechange',update))
 }catch{pwaAvailable.value=false}
}
export async function installApp(){const prompt=installPrompt.value;if(!prompt)return;await prompt.prompt();await prompt.userChoice;installPrompt.value=null}
export function applyUpdate(){if(!updateWorker.value)return;navigator.serviceWorker.addEventListener('controllerchange',()=>location.reload(),{once:true});updateWorker.value.postMessage({type:'SKIP_WAITING'})}
