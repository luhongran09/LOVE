<script setup lang="ts">
import {t,locale} from './i18n'

import { computed, onMounted, onBeforeUnmount, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import {
  Heart, Images, MapPin, Map, Settings, Plus, Search, ArrowUpRight,
  LogOut, Camera, CalendarDays, Download, Upload, Copy, Check, RefreshCw,
  Compass, Sun, Moon, LayoutGrid, List, ChevronRight, ChevronDown,
  Maximize2, Sparkles, LockKeyhole, ImagePlus, Navigation, Globe,
} from 'lucide-vue-next'
import { useAlbum } from './store'
import { api, media, type Photo, type User, type Memory } from './api'
import { theme, toggleTheme } from './theme'
import {pendingWrites} from './offline'
import MemoryMap from './components/MemoryMap.vue'
import PhotoEditor from './components/PhotoEditor.vue'
import MemoryCollection from './components/MemoryCollection.vue'
import MemoryDetail from './components/MemoryDetail.vue'
import UploadDialog from './components/UploadDialog.vue'

const album = useAlbum()
const route = useRoute()
const router = useRouter()
const mode = ref<'login' | 'register'>('login')
const name = ref('')
const password = ref('')
const invite = ref('')
const initialized = ref(true)
const authBusy = ref(false)
const authError = ref('')
const query = ref('')
const year = ref('')
const place = ref('')
const showUpload = ref(false)
const selected = ref<Photo | null>(null)
const selectedMemory = ref<Memory|null>(null)
const notice = ref('')
const pageError = ref('')
const loading = ref(false)
const layout = ref<'grid' | 'timeline'>('grid')
const mapExpanded = ref(true)
const nav = [
  { path: '/timeline', label: t('时光相册'), icon: Images },
  { path: '/places', label: t('我们的足迹'), icon: Compass },
  { path: '/map', label: t('回忆地图'), icon: Map },
  { path: '/tools', label: t('生活收藏'), icon: Sparkles },
  { path: '/settings', label: t('双人空间'), icon: Heart },
]
const places = computed(() => [...new Set(album.memories.map(p => p.city||p.place).filter(Boolean))].sort())
const years = computed(() => [...new Set(album.memories.map(p => p.date.slice(0, 4)))].sort().reverse())
const rememberedDays = computed(() => new Set(album.photos.map(p => p.taken_at)).size)
const filteredMemories = computed(()=>album.memories.filter(m=>(!year.value||m.date.startsWith(year.value))&&(!place.value||m.place===place.value||m.city===place.value)&&(!query.value||`${m.title} ${m.description} ${m.place} ${m.tags.join(' ')}`.toLowerCase().includes(query.value.toLowerCase()))))
const filtered = computed(()=>filteredMemories.value.flatMap(m=>m.photos.map(p=>({...p,mode:m.mode,memory_id:m.id,memory_title:m.title,place:m.place||m.city||p.place,latitude:p.latitude??m.latitude,longitude:p.longitude??m.longitude}))))
function openPhoto(photo:Photo){const memory=album.memories.find(m=>m.photos.some(p=>p.id===photo.id));if(memory)selectedMemory.value=memory;else selected.value=photo}
const located = computed(() => filtered.value.filter(p => p.latitude !== null && p.longitude !== null))
const latest = computed(() => album.photos[0])
const togetherDays=computed(()=>{const value=album.spaceSettings.together_date;if(typeof value!=='string'||!value)return null;const start=new Date(value+'T00:00:00').getTime();return Number.isFinite(start)?Math.max(0,Math.floor((Date.now()-start)/86400000)+1):null})
const greet = computed(() => album.members.length > 1
  ? `${album.members[0]!.name} & ${album.members[1]!.name}`
  : t('{0} 的双人空间',[album.user?.name]),
)
const heading = computed(() => ({
  '/timeline': [t('把每一刻，'), t('写成我们的故事'), t('一起看过的风景，一起度过的日常，都值得被好好收藏。')],
  '/places': [t('世界很大，'), t('有你就有方向'), t('每一个地名背后，都藏着一段只属于我们的回忆。')],
  '/map': [t('走过的路，'), t('都有爱的坐标'), t('把照片放回拍摄的地方，重新遇见那一天的我们。')],
  '/globe': [t('山海很远，'), t('一起点亮世界'), t('让每一段旅程，在地球上留下属于我们的光。')],
  '/tools': [t('不止照片，'), t('还有一起的日子'), t('城市印章、恋爱手账、共同心愿，慢慢把日子收藏起来。')],
  '/settings': [t('一方天地，'), t('刚好装下我们'), t('两个人共同收藏，也一起好好保管。')],
}[route.path] || ['', t('我们的照片日记'), '']))

async function boot() {
  await album.init()
  try {
    const setup = await api<{ initialized: boolean }>('/setup')
    initialized.value = setup.initialized
    if (!setup.initialized) mode.value = 'register'
  } catch { authError.value = t('暂时无法连接服务器，请稍后重试。') }
}
async function auth() {
  authError.value = ''
  authBusy.value = true
  try {
    album.user = await api<User>(`/auth/${mode.value}`, {
      method: 'POST', body: JSON.stringify({ name: name.value, password: password.value, invite: invite.value }),
    })
    password.value = ''
    await album.refresh()
    router.push('/timeline')
  } catch (error) { authError.value = (error as Error).message }
  finally { authBusy.value = false }
}
async function refresh() {
  loading.value = true
  pageError.value = ''
  try { await album.refresh() }
  catch (error) { pageError.value = (error as Error).message }
  finally { loading.value = false }
}
function toast(text: string) { notice.value = text; setTimeout(() => notice.value = '', 5000) }
async function uploaded(photos: Photo[],memoryID:string) {
  await refresh()
  toast(t('已收好 {0} 张回忆',[photos.length]))
  selectedMemory.value=album.memories.find(m=>m.id===memoryID)||null
}
async function logout() {
  if(pendingWrites.value.length&&!window.confirm(t('还有未同步的离线修改，退出会清除本机草稿。确定退出吗？')))return
  try {
    await album.logout()
    initialized.value = true
    mode.value = 'login'
    selected.value = null
    query.value = ''; place.value = ''; year.value = ''
  } catch (error) { pageError.value = (error as Error).message }
}
let stopSync:(()=>void)|undefined
onMounted(()=>{boot();stopSync=album.startSync()})
onBeforeUnmount(()=>stopSync?.())
</script>

<template>
  <div class="ambient" aria-hidden="true"><i></i><i></i><div class="ambient-grid"></div></div>

  <div v-if="!album.ready" class="loading-screen">
    <span class="loading-orbit"><Compass :size="34" /></span>
    <p>{{t('正在打开我们的相册…')}}</p>
  </div>

  <main v-else-if="!album.user" class="auth-page">
    <header class="auth-header">
      <a href="#/timeline" class="brand"><span class="brand-icon"><Compass :size="25" /></span><span>{{t('足迹')}}<small>FOOTPRINT · OUR STORY</small></span></a>
      <button class="icon theme-toggle" :aria-label="theme === 'dark' ? t('切换浅色模式') : t('切换深色模式')" @click="toggleTheme"><Sun v-if="theme === 'dark'" :size="19" /><Moon v-else :size="19" /></button>
    </header>
    <section class="auth-story">
      <div class="auth-copy">
        <span class="eyebrow"><span></span> A WORLD OF OUR OWN</span>
        <h1>{{t('山海很远，')}}<br>{{t('还好，')}}<em>{{t('我们一起。')}}</em></h1>
        <p>{{t('收藏走过的路，也收藏彼此。')}}<br>{{t('把每一张照片，都变成回得去的时光。')}}</p>
        <div class="story-art" aria-hidden="true">
          <div class="orbit orbit-one"></div><div class="orbit orbit-two"></div>
          <span class="art-coordinate">30°15′ N &nbsp; 120°09′ E</span>
          <div class="art-postcard postcard-back"><MapPin :size="29" /><span>{{t('去看更大的世界')}}</span></div>
          <div class="art-postcard postcard-front">
            <div class="art-landscape"><span class="art-sun"></span><span class="art-mountain far"></span><span class="art-mountain near"></span><span class="art-water"></span><span class="art-spark">✦</span></div>
            <div class="postcard-caption"><span>{{t('下一站，和你。')}}</span><Heart :size="16" /></div>
          </div>
          <span class="art-pin pin-one"><Heart :size="16" /></span><span class="art-pin pin-two"><Navigation :size="16" /></span>
          <span class="art-dashed-path"></span>
        </div>
        <div class="auth-promise"><LockKeyhole :size="14" />{{t('只属于我们两个人的私密空间')}}<span>·</span>{{t('照片由你们自己保管')}}</div>
      </div>
    </section>
    <section class="auth-panel">
      <form class="auth-form glass" @submit.prevent="auth">
        <span class="auth-heart"><Heart :size="24" /></span>
        <span class="eyebrow">OUR LITTLE UNIVERSE</span>
        <h2>{{ mode === 'login' ? t('欢迎回到，我们的世界') : initialized ? t('另一半，正在等你') : t('从这里，写下我们的故事') }}</h2>
        <p class="muted">{{ mode === 'login' ? t('那些闪闪发光的日常，都在这里。') : initialized ? t('用 TA 发来的邀请码，一起留下新的足迹。') : t('创建一个空间，装下两个人的美好瞬间。') }}</p>
        <div class="auth-tabs">
          <button type="button" :class="{ active: mode === 'login' }" @click="mode = 'login'; authError = ''">{{t('登录相册')}}</button>
          <button type="button" :class="{ active: mode === 'register' }" @click="mode = 'register'; authError = ''">{{ initialized ? t('接受邀请') : t('创建空间') }}</button>
        </div>
        <label>{{t('你的名字')}}<input v-model="name" autocomplete="username" required minlength="2" maxlength="30" :placeholder="t('用一个熟悉的名字')" /></label>
        <label>{{t('密码')}}<input v-model="password" type="password" :autocomplete="mode === 'login' ? 'current-password' : 'new-password'" required minlength="8" maxlength="72" :placeholder="t('至少 8 位字符')" /></label>
        <label v-if="mode === 'register' && initialized">{{t('伴侣邀请码')}}<input v-model="invite" required autocomplete="off" :placeholder="t('粘贴 TA 发来的邀请码')" /></label>
        <p v-if="authError" class="error" role="alert">{{ authError }}</p>
        <button class="primary wide" :disabled="authBusy">{{ authBusy ? t('正在打开…') : mode === 'login' ? t('打开我们的相册') : initialized ? t('加入双人空间') : t('创建我们的空间') }}<ArrowUpRight :size="18" /></button>
        <p class="auth-hint"><LockKeyhole :size="12" />{{t('每一份回忆，仅彼此可见')}}</p>
      </form>
    </section>
    <footer class="auth-footer">FOOTPRINT <span>MADE FOR OUR EVERYDAY ADVENTURES</span></footer>
  </main>

  <div v-else class="app-shell">
    <header class="site-header">
      <div class="header-inner">
        <RouterLink to="/timeline" class="brand"><span class="brand-icon"><Compass :size="25" /></span><span>{{t('足迹')}}<small>FOOTPRINT · OUR STORY</small></span></RouterLink>
        <nav class="main-nav" :aria-label="t('主导航')">
          <RouterLink v-for="item in nav" :key="item.path" :to="item.path"><component :is="item.icon" :size="16" /><span>{{ item.label }}</span></RouterLink>
        </nav>
        <div class="header-actions">
          <span class="private-badge"><span></span>{{t('双人私密空间')}}</span>
          <button class="icon theme-toggle" :aria-label="theme === 'dark' ? t('切换浅色模式') : t('切换深色模式')" @click="toggleTheme"><Sun v-if="theme === 'dark'" :size="18" /><Moon v-else :size="18" /></button>
          <RouterLink to="/settings" class="header-avatar" :aria-label="t('{0}，打开双人空间',[album.user.name])">{{ album.user.name.slice(0, 1) }}</RouterLink>
          <button class="icon logout" :aria-label="t('退出登录')" :title="t('退出登录')" @click="logout"><LogOut :size="17" /></button>
        </div>
      </div>
    </header>

    <main class="content">
      <section class="page-heading">
        <div>
          <div class="eyebrow"><Sparkles :size="13" /> LITTLE MOMENTS, BIG MEMORIES</div>
          <h1>{{ heading[0] }}<em>{{ heading[1] }}</em><span class="heading-dot">.</span></h1>
          <p>{{ heading[2] }}</p>
        </div>
        <button class="primary" @click="showUpload = true"><Plus :size="18" />{{t('添加回忆')}}</button>
      </section>
      <p v-if="pageError" class="error" role="alert">{{ pageError }}</p>

      <section v-if="route.path === '/timeline'" class="overview-section">
        <div class="overview-map glass" :class="{ collapsed: !mapExpanded, 'empty-map': !located.length }">
          <div class="overview-map-heading">
            <span><MapPin :size="15" />{{t('我们的足迹地图')}}<i>{{t('{0} 张照片有坐标', [located.length])}}</i></span>
            <div><RouterLink to="/map" class="icon" :aria-label="t('打开完整地图')"><Maximize2 :size="15" /></RouterLink><button class="icon" :aria-label="mapExpanded ? t('收起地图') : t('展开地图')" :aria-expanded="mapExpanded" @click="mapExpanded = !mapExpanded"><ChevronDown :size="17" :class="{ flipped: !mapExpanded }" /></button></div>
          </div>
          <MemoryMap v-if="mapExpanded" :photos="filtered" @open="openPhoto" />
          <div v-if="mapExpanded && !located.length" class="map-intro"><span class="map-intro-icon"><Navigation :size="19" /></span><div><strong>{{t('世界这么大，下一站一起出发')}}</strong><p>{{t('给照片标记位置，就能在这里点亮共同的足迹。')}}</p></div></div>
        </div>
        <div class="couple-card glass" :class="{ 'has-photo': latest }">
          <img v-if="latest" class="couple-backdrop" :src="media(latest.id)" alt="" />
          <div class="couple-orbit" aria-hidden="true"></div>
          <div class="couple-card-top"><span><Heart :size="13" /> JUST THE TWO OF US</span><Sparkles :size="18" /></div>
          <div class="avatar-pair"><span>{{ album.members[0]?.name.slice(0, 1) }}</span><Heart :size="16" /><span>{{ album.members[1]?.name.slice(0, 1) || '♡' }}</span></div>
          <h2>{{ greet }}</h2>
          <p>{{ togetherDays!==null?t('在一起的第 {0} 天',[togetherDays]):album.members.length > 1 ? t('两个人的小日子，值得被认真收藏。') : t('故事的另一位主角，还在等你邀请。') }}</p>
          <button v-if="album.members.length < 2" class="couple-link" @click="router.push('/settings')">{{t('邀请 TA 加入')}}<ChevronRight :size="15" /></button>
          <button v-else class="couple-link" @click="showUpload = true">{{t('再收藏一个心动瞬间')}}<ChevronRight :size="15" /></button>
          <div class="couple-card-bottom"><span class="live-dot"></span>{{ album.photos.length ? t('已珍藏 {0} 个美好瞬间',[album.photos.length]) : t('我们的故事，未完待续') }}<Heart :size="13" /></div>
        </div>
      </section>

      <section v-if="['/timeline','/places','/map'].includes(route.path)" class="stats-strip glass">
        <div><span class="stat-icon teal"><Images :size="21" /></span><span><strong>{{ album.photos.length }}<small>{{t('张')}}</small></strong><label>{{t('珍藏的照片')}}</label></span></div>
        <div><span class="stat-icon blue"><MapPin :size="21" /></span><span><strong>{{ places.length }}<small>{{t('处')}}</small></strong><label>{{t('共同的足迹')}}</label></span></div>
        <div><span class="stat-icon rose"><CalendarDays :size="21" /></span><span><strong>{{ rememberedDays }}<small>{{t('天')}}</small></strong><label>{{t('被记住的日子')}}</label></span></div>
        <p class="stats-quote"><Heart :size="15" /><span>{{t('风景会变，')}}<br>{{t('与你有关的都想留住。')}}</span></p>
      </section>

      <section v-if="route.path === '/timeline'" class="explorer-hub" :aria-label="t('回忆快捷入口')">
        <RouterLink to="/map" class="hub-card hub-teal"><span class="hub-icon"><Map :size="23" /></span><div><h3>{{t('足迹地图')}}</h3><p>{{t('重走一起走过的路')}}</p></div><ArrowUpRight :size="17" /></RouterLink>
        <RouterLink to="/globe" class="hub-card hub-blue"><span class="hub-icon"><Globe :size="23" /></span><div><h3>{{t('3D 点亮地球')}}</h3><p>{{t('一起走过的世界')}}</p></div><ArrowUpRight :size="17" /></RouterLink>
        <button class="hub-card hub-rose" @click="layout = layout === 'grid' ? 'timeline' : 'grid'"><span class="hub-icon"><CalendarDays :size="23" /></span><div><h3>{{t('时光轨迹')}}</h3><p>{{ layout === 'grid' ? t('沿着时间，重温心动') : t('切换回照片墙') }}</p></div><ArrowUpRight :size="17" /></button>
        <RouterLink to="/tools" class="hub-card hub-amber"><span class="hub-icon"><Sparkles :size="23" /></span><div><h3>{{t('生活收藏馆')}}</h3><p>{{t('护照、手账和小心愿')}}</p></div><ArrowUpRight :size="17" /></RouterLink>
      </section>

      <template v-if="['/timeline','/places','/map'].includes(route.path)">
        <div class="collection-heading">
          <h2><span class="section-dot"></span>{{ route.path === '/timeline' ? t('回忆收藏集') : route.path === '/places' ? t('城市里的我们') : t('回忆的坐标') }}<span>{{t('{0} 张照片', [filtered.length])}}</span></h2>
          <div v-if="route.path === '/timeline'" class="view-switch" :aria-label="t('相册显示方式')"><button :class="{ active: layout === 'grid' }" :aria-pressed="layout === 'grid'" :aria-label="t('照片墙')" @click="layout = 'grid'"><LayoutGrid :size="15" />{{t('照片墙')}}</button><button :class="{ active: layout === 'timeline' }" :aria-pressed="layout === 'timeline'" :aria-label="t('时间轴')" @click="layout = 'timeline'"><List :size="15" />{{t('时间轴')}}</button></div>
        </div>
        <div class="filters">
          <label class="search"><Search :size="16" /><input v-model="query" :placeholder="t('搜索回忆、地点或备注…')" :aria-label="t('搜索回忆')" /></label>
          <select v-model="year" :aria-label="t('按年份筛选')"><option value="">{{t('所有年份')}}</option><option v-for="y in years" :key="y">{{ y }}</option></select>
          <select v-model="place" :aria-label="t('按地点筛选')"><option value="">{{t('所有地点')}}</option><option v-for="p in places" :key="p">{{ p }}</option></select>
          <button class="icon refresh-button" :disabled="loading" :aria-label="t('刷新照片')" @click="refresh"><RefreshCw :size="16" :class="{ spinning: loading }" /></button>
        </div>
      </template>

      <RouterView v-slot="{ Component }"><component :is="Component" :query="query" :year="year" :place="place" :layout="layout" @open="selectedMemory=$event" @upload="showUpload=true" @clear="query='';year='';place=''" @place="place=$event;router.push('/timeline')" /></RouterView>
      <footer class="page-footer"><span><Compass :size="15" />{{t('足迹')}}<small>FOOTPRINT</small></span><p>{{t('和你一起，把日子过成值得收藏的故事。')}}</p><span><Heart :size="12" /> MADE FOR TWO</span></footer>
    </main>

    <UploadDialog v-if="showUpload" @close="showUpload = false" @uploaded="uploaded" />
    <MemoryDetail v-if="selectedMemory" :key="selectedMemory.id" :memory="selectedMemory" @close="selectedMemory=null" @changed="refresh" />
    <PhotoEditor v-if="selected" :key="selected.id" :photo="selected" @close="selected = null" @changed="refresh" />
    <div v-if="notice" class="toast" role="status"><Check :size="17" />{{ notice }}</div>
  </div>
</template>
