<script setup lang="ts">
import {t,locale} from '../i18n'

import { computed,ref,watch,onBeforeUnmount } from 'vue'
import { Heart,Compass,Utensils,Layers,MapPin,CalendarDays,Merge,Check,X } from 'lucide-vue-next'
import { useAlbum } from '../store'
import { api,modes,type Memory } from '../api'
import MemoryMosaic from './MemoryMosaic.vue'
const props=defineProps<{query:string;year:string;place:string;layout:'grid'|'timeline'}>()
const emit=defineEmits<{open:[memory:Memory];upload:[];clear:[]}>()
const album=useAlbum(),mode=ref(''),selecting=ref(false),chosen=ref<string[]>([]),merging=ref(false),error=ref('')
const items=ref<Memory[]>([]),page=ref(1),total=ref(0),loading=ref(false),pageSize=30
let generation=0,controller:AbortController|undefined
async function load(){
 const version=++generation;controller?.abort();controller=new AbortController();loading.value=true;error.value=''
 try{const params=new URLSearchParams({page:String(page.value),per_page:String(pageSize),q:props.query,year:props.year,place:props.place,mode:mode.value});const result=await api<{memories:Memory[];total:number}>(`/memories?${params}`,{signal:controller.signal});if(version!==generation)return
 if(page.value>1&&(page.value-1)*pageSize>=result.total){page.value=Math.max(1,Math.ceil(result.total/pageSize));return}
 items.value=result.memories;total.value=result.total
 }catch(e){if(version===generation&&(e as Error).name!=='AbortError')error.value=(e as Error).message}finally{if(version===generation)loading.value=false}
}
watch(()=>[props.query,props.year,props.place,mode.value],()=>{if(page.value===1)load();else page.value=1})
watch(page,()=>{chosen.value=[];load()})
watch(()=>album.memories,load,{immediate:true})
onBeforeUnmount(()=>{generation++;controller?.abort()})
function changePage(next:number){page.value=next;document.querySelector('.memory-toolbar')?.scrollIntoView({behavior:'smooth',block:'start'})}

const groups=computed(()=>{const map:Record<string,Memory[]>={};for(const m of items.value)(map[m.date.slice(0,7)]??=[]).push(m);return Object.entries(map)})
function toggle(id:string){chosen.value=chosen.value.includes(id)?chosen.value.filter(x=>x!==id):[...chosen.value,id]}
async function merge(){const memories=album.memories.filter(m=>chosen.value.includes(m.id));if(memories.length<2)return;merging.value=true;error.value='';try{const first=memories[0]!;const ids=memories.flatMap(m=>m.photos.map(p=>p.id));const result=await api<{id:string}>('/memories',{method:'POST',body:JSON.stringify({...first,id:undefined,title:t('{0} · 合集',[first.title.slice(0,110)]),description:memories.map(m=>`${m.title}\n${m.description}`).join('\n\n'),tags:[...new Set(memories.flatMap(m=>m.tags))],photo_ids:ids})});await album.refresh();chosen.value=[];selecting.value=false;const created=album.memories.find(m=>m.id===result.id);if(created)emit('open',created)}catch(e){error.value=(e as Error).message}finally{merging.value=false}}
</script>
<template>
 <div class="memory-toolbar"><div class="memory-modes"><button v-for="item in [{key:'',label:t('全部回忆'),icon:Layers},{key:'travel',label:t('旅行探索'),icon:Compass},{key:'food',label:t('美食寻味'),icon:Utensils},{key:'love',label:t('专属回忆'),icon:Heart}]" :key="item.key" :class="{active:mode===item.key}" @click="mode=item.key"><component :is="item.icon" :size="14"/>{{item.label}}</button></div><button class="text-button" @click="selecting=!selecting;chosen=[]"><Merge :size="14"/>{{selecting?t('取消选择'):t('选择并合并')}}</button></div>
 <div v-if="selecting" class="merge-bar"><span>{{t('已选择 {0} 段回忆；合并后保留每张照片的拍摄信息。', [chosen.length])}}</span><button class="primary" :disabled="chosen.length<2||merging" @click="merge"><Merge :size="14"/>{{merging?t('合并中…'):t('合并为一段回忆')}}</button></div>
 <p v-if="error" class="error" role="alert">{{error}}</p>
 <p v-if="loading" class="muted" role="status">{{t('正在打开回忆…')}}</p>
 <div class="collection memory-collection" :class="{'timeline-layout':layout==='timeline'}">
  <section v-for="[month,memories] in groups" :key="month" class="month-section"><div class="month-heading"><h2><CalendarDays :size="13"/>{{t('{0} 年 {1} 月', [month.slice(0,4),Number(month.slice(5,7))])}}</h2><span>{{t('{0} 段回忆 · {1} 张照片', [memories.length,memories.reduce((n,m)=>n+m.photos.length,0)])}}</span><i></i></div>
   <div class="photo-grid"><div v-for="memory in memories" :key="memory.id" class="memory-entry" :class="{chosen:chosen.includes(memory.id)}" :style="{'--memory-color':modes[memory.mode]?.color}"><button class="photo-card memory-card" @click="selecting?toggle(memory.id):emit('open',memory)"><MemoryMosaic :memory="memory" :compact="layout==='timeline'"/><div class="photo-caption"><span class="memory-type">{{modes[memory.mode]?.label}}<span>{{memory.date}}</span></span><h3>{{memory.title}}</h3><p v-if="memory.description">{{memory.description}}</p><div v-if="memory.tags.length" class="memory-tags"><span v-for="tag in memory.tags.slice(0,4)" :key="tag"># {{tag}}</span></div><div class="photo-meta"><span><MapPin :size="12"/>{{memory.place||memory.city||t('还没有标记地点')}}</span><span v-if="memory.rating">{{'★'.repeat(memory.rating)}}</span><Heart v-else :size="13"/></div></div></button><button v-if="selecting" class="memory-select" :aria-label="t('选择{0}',[memory.title])" :aria-pressed="chosen.includes(memory.id)" @click="toggle(memory.id)"><Check v-if="chosen.includes(memory.id)" :size="16"/></button></div></div>
  </section>
  <section v-if="!loading&&!error&&!items.length" class="empty-state glass"><span class="empty-icon"><Heart :size="32"/></span><span class="eyebrow">OUR STORY STARTS HERE</span><h2>{{album.memories.length?t('还没找到这段回忆'):t('一段回忆，可以有很多个瞬间')}}</h2><p>{{album.memories.length?t('试试其他分类、时间、地点或关键词。'):t('把旅行、晚餐或日常的多张照片，收进同一个故事。')}}</p><button v-if="!album.memories.length" class="primary" @click="emit('upload')">{{t('添加第一段回忆')}}</button><button v-else class="secondary" @click="mode='';emit('clear')"><X :size="14"/>{{t('清除筛选')}}</button></section>
 </div>
 <nav v-if="total>pageSize" class="memory-pagination" :aria-label="t('回忆分页')"><button class="secondary" :disabled="loading||page<=1" @click="changePage(page-1)">{{t('上一页')}}</button><span>{{t('第 {0} / {1} 页 · 共 {2} 段回忆', [page,Math.ceil(total/pageSize),total])}}</span><button class="secondary" :disabled="loading||page*pageSize>=total" @click="changePage(page+1)">{{t('下一页')}}</button></nav>
</template>
