<script setup lang="ts">
import {t,locale} from '../i18n'

import { computed,onMounted,onBeforeUnmount,ref,watch } from 'vue'
import { Globe,RotateCw,MapPin,Pause } from 'lucide-vue-next'
import { useAlbum } from '../store'
import { worldCountries,countryAt } from '../offlineMap'
import { modes,type Memory } from '../api'
import MemoryMap from '../components/MemoryMap.vue'
const emit=defineEmits<{open:[memory:Memory]}>(),album=useAlbum(),host=ref<HTMLDivElement>(),error=ref(''),loading=ref(true),rotating=ref(!matchMedia('(prefers-reduced-motion: reduce)').matches)
let globe:any,resize:ResizeObserver|undefined,disposed=false
const locations=computed(()=>album.memories.flatMap(memory=>{const photo=memory.photos.find(p=>p.latitude!==null&&p.longitude!==null);const lat=memory.latitude??photo?.latitude,lon=memory.longitude??photo?.longitude;return lat!=null&&lon!=null?[{latitude:lat,longitude:lon,memory}]:[]}))
const countries=computed(()=>new Set(locations.value.map(p=>countryAt(p.latitude,p.longitude)).filter(Boolean)))
const cities=computed(()=>new Set(album.memories.map(m=>m.city||m.place).filter(Boolean)))
function render(){if(!globe)return;globe.pointsData(locations.value).polygonsData(worldCountries.features);globe.polygonCapColor((f:any)=>countries.value.has(String(f.id))?'#248e91':'#142d40')}
function focus(global=false,duration=900){globe?.pointOfView(global?{lat:20,lng:0,altitude:2.6}:{lat:33,lng:106,altitude:1.8},duration)}
function rotate(){rotating.value=!rotating.value;if(globe)globe.controls().autoRotate=rotating.value}
onMounted(async()=>{try{const {default:GlobeGL}=await import('globe.gl');if(disposed)return;globe=GlobeGL()(host.value!).width(host.value!.clientWidth).height(540).backgroundColor('#080d17').showAtmosphere(true).atmosphereColor('#27b6b4').atmosphereAltitude(.2).polygonsTransitionDuration(400).polygonAltitude((f:any)=>countries.value.has(String(f.id))?.035:.008).polygonSideColor(()=> '#123342').polygonStrokeColor(()=> '#2e576c').polygonLabel((f:any)=>String(f.properties?.name||'').replace(/[<>&"']/g,'')).pointLat('latitude').pointLng('longitude').pointRadius(.24).pointAltitude(.045).pointColor((p:any)=>modes[p.memory.mode as keyof typeof modes].color).pointLabel((p:any)=>p.memory.title.replace(/[<>&"']/g,'')).onPointClick((p:any)=>emit('open',p.memory));globe.controls().autoRotate=rotating.value;globe.controls().autoRotateSpeed=.35;globe.controls().enableZoom=true;globe.globeMaterial().color.set('#10283a');render();focus(false,0);resize=new ResizeObserver(()=>{if(globe&&host.value)globe.width(host.value.clientWidth)});resize.observe(host.value!)}catch{error.value=t('当前设备无法启用 3D 渲染，已为你切换为二维足迹地图。')}finally{loading.value=false}})
watch(locations,render,{deep:true});onBeforeUnmount(()=>{disposed=true;resize?.disconnect();globe?.pauseAnimation();globe?._destructor?.()})
</script>
<template><section class="globe-panel glass"><div class="globe-toolbar"><span><Globe :size="18"/>{{t('一起点亮的世界')}}</span><div><button class="secondary" :disabled="loading||!!error" @click="focus()">{{t('国内视角')}}</button><button class="secondary" :disabled="loading||!!error" @click="focus(true)">{{t('全球视角')}}</button><button class="icon" :disabled="loading||!!error" :aria-label="rotating?t('暂停地球旋转'):t('开启地球旋转')" @click="rotate"><Pause v-if="rotating" :size="16"/><RotateCw v-else :size="16"/></button></div></div><div v-show="!error" ref="host" class="globe-canvas"></div><p v-if="loading" class="globe-loading">{{t('正在展开我们的世界…')}}</p><template v-if="error"><p class="small muted">{{error}}</p><div class="globe-fallback"><MemoryMap :photos="album.photos" @open="photo=>{const memory=album.memories.find(m=>m.photos.some(p=>p.id===photo.id));if(memory)emit('open',memory)}"/></div></template><div class="globe-stats"><span><strong>{{countries.size}}</strong>{{t('个有足迹的国家／地区轮廓')}}</span><span><strong>{{cities.size}}</strong>{{t('个记录地点')}}</span><span><strong>{{locations.length}}</strong>{{t('段带坐标的回忆')}}</span></div><p class="small muted">{{t('拖动旋转，滚轮缩放，点击彩色标记查看回忆。区域轮廓依据照片坐标点亮；地理边界来自离线低精度数据。')}}</p></section></template>
