import { defineConfig,type Plugin } from 'vite'
import vue from '@vitejs/plugin-vue'
function offlineShell():Plugin{return {name:'footprint-offline-shell',apply:'build',generateBundle(_options,bundle){
 const assets=Object.keys(bundle).filter(path=>path.startsWith('assets/')).map(path=>'/'+path)
 const files=['/','/manifest.webmanifest','/icons/footprint.svg',...assets]
 const cache=`footprint-shell-${Date.now()}-${assets.length}`
 this.emitFile({type:'asset',fileName:'sw.js',source:`
const CACHE=${JSON.stringify(cache)},FILES=${JSON.stringify(files)};
self.addEventListener('install',event=>event.waitUntil(caches.open(CACHE).then(cache=>cache.addAll(FILES))));
self.addEventListener('message',event=>{if(event.data?.type==='SKIP_WAITING')self.skipWaiting()});
self.addEventListener('activate',event=>event.waitUntil((async()=>{const names=(await caches.keys()).filter(name=>name.startsWith('footprint-shell-')).sort().reverse();await Promise.all(names.slice(2).map(name=>caches.delete(name)));await self.clients.claim()})()));
self.addEventListener('fetch',event=>{
 const url=new URL(event.request.url);
 if(event.request.method!=='GET'||url.origin!==self.location.origin||url.pathname.startsWith('/api/'))return;
 if(event.request.mode==='navigate'){event.respondWith(fetch(event.request).catch(()=>caches.open(CACHE).then(cache=>cache.match('/'))));return}
 if(!url.pathname.startsWith('/assets/')&&!FILES.includes(url.pathname))return;
 event.respondWith((async()=>{for(const name of (await caches.keys()).filter(name=>name.startsWith('footprint-shell-')).sort().reverse()){const response=await (await caches.open(name)).match(event.request);if(response)return response}return fetch(event.request)})());
});`})
}}}
export default defineConfig({ plugins: [vue(),offlineShell()], server: { proxy: { '/api': 'http://127.0.0.1:8080' } } })
