<script setup lang="ts">
import {t,locale} from '../i18n'

import { onMounted, onBeforeUnmount, watch, ref } from 'vue'
import { RotateCw,Focus } from 'lucide-vue-next'
import L from 'leaflet'
import 'leaflet/dist/leaflet.css'
import { thumbnail, modes, type Photo, type MemoryMode } from '../api'
import { addOfflineMap } from '../offlineMap'
import { toAmap,fromAmap,type Coordinates } from '../mapCoordinates'

type Provider='amap'|'osm'|'offline'
type MapPhoto=Photo & {mode?:MemoryMode;memory_id?:string;memory_title?:string}
const props=defineProps<{photos:MapPhoto[];pick?:boolean;selected?:Coordinates|null;route?:Coordinates[]}>()
const emit=defineEmits<{open:[photo:Photo];select:[coords:Coordinates]}>()
const host=ref<HTMLDivElement>(),provider=ref<Provider>('amap'),active=ref<Provider>('amap'),status=ref<'loading'|'ready'|'partial'|'offline'>('loading')
try{const saved=localStorage.getItem('footprint-map-provider');if(saved==='amap'||saved==='osm'||saved==='offline')provider.value=saved}catch{}
let map:L.Map|undefined,layer:L.LayerGroup|undefined,tiles:L.TileLayer|undefined,resize:ResizeObserver|undefined,tileCheck:ReturnType<typeof setTimeout>|undefined,version=0
let offlineVisibility:((visible:boolean)=>void)|undefined
const names={amap:t('高德地图'),osm:'OpenStreetMap',offline:t('离线轮廓')}
function display(coords:Coordinates):Coordinates{return active.value==='amap'?toAmap(coords):coords}
function stored(coords:Coordinates):Coordinates{return active.value==='amap'?fromAmap(coords):coords}
function coordinates(photo:MapPhoto):Coordinates{return display([photo.latitude!,photo.longitude!])}
function valid(photo:MapPhoto){return photo.latitude!=null&&photo.longitude!=null&&Number.isFinite(photo.latitude)&&Number.isFinite(photo.longitude)}
function fit(){
 if(!map)return
 const points=props.photos.filter(valid).map(coordinates)
 if(props.selected)points.push(display(props.selected))
 if(points.length)map.fitBounds(L.latLngBounds(points),{padding:[48,48],maxZoom:active.value==='offline'&&!props.pick?6:15,animate:false})
 else map.setView(display([34.3416,108.9398]),5)
}
function paint(){
 if(!map||!layer)return
 layer.clearLayers()
 if(props.route&&props.route.length>1)L.polyline(props.route.map(display),{color:'#36b8c4',weight:3,dashArray:'5 7',opacity:.8}).addTo(layer)
 const groups=new Map<string,MapPhoto[]>()
 for(const photo of props.photos){
  if(!valid(photo))continue
  const point=coordinates(photo),pixel=map.project(point,map.getZoom())
  const key=map.getZoom()>=16?`${point[0].toFixed(5)},${point[1].toFixed(5)}`:`${Math.floor(pixel.x/50)},${Math.floor(pixel.y/50)}`
  const group=groups.get(key)||[];group.push(photo);groups.set(key,group)
 }
 for(const photos of groups.values()){
  const first=photos[0]!,memories=[...new Map(photos.map(p=>[p.memory_id||p.id,p])).values()],count=memories.length
  const position:Coordinates=[photos.reduce((n,p)=>n+p.latitude!,0)/photos.length,photos.reduce((n,p)=>n+p.longitude!,0)/photos.length]
  const mode=first.mode||'travel',glyph=mode==='food'?'🍜':mode==='love'?'💕':'✈'
  const label=count>1?t('{0} 段回忆 · {1} 张照片',[count,photos.length]):`${first.memory_title||first.title||t('一段回忆')}${first.place?' · '+first.place:''}`
  const icon=L.divIcon({className:count>1?'footprint-cluster':`footprint-pin-wrap pin-${mode}`,html:count>1?`<span>${count}</span>`:`<span class="footprint-pin"><span>${glyph}</span></span>${photos.length>1?`<small>${photos.length}</small>`:''}`,iconSize:count>1?[42,42]:[38,44],iconAnchor:count>1?[21,21]:[19,38]})
  const marker=L.marker(display(position),{icon,keyboard:true,title:label}).bindTooltip(label).addTo(layer)
  const popup=document.createElement('div');popup.className='marker-gallery'
  const heading=document.createElement('strong');heading.textContent=count>1?t('{0} 段回忆 · {1} 张照片',[count,photos.length]):first.memory_title||first.title||t('我们的回忆');popup.append(heading)
  const grid=document.createElement('div');grid.className='marker-gallery-grid'
  for(const item of memories){
   const button=document.createElement('button');button.type='button'
   const image=document.createElement('img');image.src=thumbnail(item.id);image.alt=item.memory_title||item.title||t('我们的照片');image.loading='lazy'
   const caption=document.createElement('span');caption.textContent=item.memory_title||item.title||item.taken_at
   const detail=document.createElement('small');detail.textContent=`${modes[item.mode||'travel'].label} · ${item.taken_at}${item.place?' · '+item.place:''}`
   button.append(image,caption,detail);button.addEventListener('click',()=>{map?.closePopup();emit('open',item)});grid.append(button)
  }
  popup.append(grid);marker.bindPopup(popup,{maxWidth:300,className:'memory-popup'})
  if(count>1)marker.on('click',()=>{
   if(!map||map.getZoom()>=16)return
   const bounds=L.latLngBounds(photos.map(coordinates));const spread=map.latLngToContainerPoint(bounds.getNorthEast()).distanceTo(map.latLngToContainerPoint(bounds.getSouthWest()))
   if(spread>8){map.closePopup();map.fitBounds(bounds,{padding:[65,65],maxZoom:Math.min(16,map.getZoom()+3)})}
  })
 }
 if(props.selected)L.circleMarker(display(props.selected),{radius:9,color:'#fff',weight:3,fillColor:'#2dd4bf',fillOpacity:1}).addTo(layer)
}
function loadProvider(next:Provider,allowFallback=true){
 if(!map)return
 const previousCenter=map.getCenter(),center=stored([previousCenter.lat,previousCenter.lng]),zoom=map.getZoom(),current=++version
 clearTimeout(tileCheck);tiles?.off();tiles?.remove();tiles=undefined;active.value=next
 map.setView(display(center),zoom,{animate:false});paint();offlineVisibility?.(true)
 if(next==='offline'){status.value='offline';return}
 status.value='loading';let successes=0,errors=0
 function fallback(){if(current!==version||successes>0)return;if(allowFallback&&next==='amap'){loadProvider('osm',false)}else{loadProvider('offline',false)}}
 const url=next==='amap'?'https://webrd0{s}.is.autonavi.com/appmaptile?lang=zh_cn&size=1&scale=1&style=8&x={x}&y={y}&z={z}':'https://tile.openstreetmap.org/{z}/{x}/{y}.png'
 tiles=L.tileLayer(url,{subdomains:['1','2','3','4'],maxZoom:next==='amap'?18:19,attribution:next==='amap'?t('&copy; <a href="https://www.amap.com/" target="_blank" rel="noopener">高德地图</a>'):'&copy; <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a> contributors'})
 tiles.on('tileerror',()=>{if(++errors>=6&&!successes)fallback()})
 function checkView(){clearTimeout(tileCheck);tileCheck=setTimeout(()=>{if(current!==version)return;if(successes>0)status.value='partial';else fallback()},10000)}
 tiles.on('loading',()=>{if(current!==version)return;successes=0;errors=0;status.value='loading';offlineVisibility?.(true);checkView()})
 tiles.on('tileload',()=>{if(current!==version)return;successes++})
 tiles.on('load',()=>{if(current!==version)return;clearTimeout(tileCheck);if(successes>0){const failed=[...tiles!.getContainer()!.querySelectorAll('img')].some(image=>image.complete&&image.naturalWidth===0);status.value=failed?'partial':'ready'}else fallback()})
 checkView();tiles.addTo(map)
}
function choose(){try{localStorage.setItem('footprint-map-provider',provider.value)}catch{};loadProvider(provider.value)}
onMounted(()=>{
 active.value=provider.value
 map=L.map(host.value!,{scrollWheelZoom:false,zoomControl:false,zoomAnimation:false,markerZoomAnimation:false,minZoom:2,worldCopyJump:true}).setView(display([34.3416,108.9398]),5)
 map.attributionControl.setPrefix(false);offlineVisibility=addOfflineMap(map);L.control.zoom({position:'bottomright'}).addTo(map);layer=L.layerGroup().addTo(map)
 map.on('zoomend',paint)
 map.on('click',(event:L.LeafletMouseEvent)=>{if(props.pick){const point=stored([event.latlng.lat,event.latlng.lng]);emit('select',[Number(point[0].toFixed(6)),Number(point[1].toFixed(6))])}})
 resize=new ResizeObserver(()=>map?.invalidateSize());resize.observe(host.value!);fit();loadProvider(provider.value)
})
watch(()=>props.photos,()=>{paint();fit()},{deep:true})
watch(()=>props.selected,()=>{paint();if(props.selected&&map)map.panTo(display(props.selected))},{deep:true})
watch(()=>props.route,paint,{deep:true})
onBeforeUnmount(()=>{version++;clearTimeout(tileCheck);resize?.disconnect();tiles?.off();map?.remove()})
</script>
<template><div class="map-surface" :class="{picking:pick}"><div ref="host" class="memory-map" :aria-label="t('照片地点地图')"></div><div class="map-actions"><select v-model="provider" :aria-label="t('地图底图')" @change="choose"><option value="amap">{{t('高德地图')}}</option><option value="osm">OpenStreetMap</option><option value="offline">{{t('离线轮廓')}}</option></select><button type="button" :aria-label="t('重试加载地图')" :title="t('重试加载地图')" @click="choose"><RotateCw :size="15"/></button><button type="button" :aria-label="t('查看全部足迹')" :title="t('查看全部足迹')" @click="fit"><Focus :size="16"/></button></div><div v-if="!pick&&photos.some(valid)" class="map-legend"><span class="travel">{{t('✈ 旅行')}}</span><span class="food">{{t('🍜 美食')}}</span><span class="love">{{t('💕 情侣')}}</span></div><span v-if="status==='loading'" class="map-network-note" role="status">{{t('正在加载{0}…', [names[active]])}}</span><span v-else-if="status==='offline'" class="map-network-note" role="status">{{t('{0}；不含街道细节，可切换底图或重试。', [provider==='offline'?t('当前使用离线轮廓'):t('在线底图未能加载，已显示离线轮廓')])}}</span><span v-else-if="status==='partial'" class="map-network-note" role="status">{{t('部分街道底图未加载，空缺处显示离线轮廓，可点击重试。')}}</span><span v-else-if="provider!==active" class="map-network-note" role="status">{{t('{0}暂不可用，已切换至{1}。', [names[provider],names[active]])}}</span></div></template>
