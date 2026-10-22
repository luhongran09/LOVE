import L from 'leaflet'
import type { FeatureCollection, Geometry, Position } from 'geojson'
import source from './assets/world-110m.json'

interface TopologyGeometry {
  type: 'Polygon' | 'MultiPolygon'
  arcs: number[][] | number[][][]
}
interface Topology {
  transform: { scale: [number, number]; translate: [number, number] }
  arcs: [number, number][][]
  objects: { land: { geometries: TopologyGeometry[] } }
}

// Natural Earth land outlines, redistributed by world-atlas 2.0.2.
// Decode once, then reuse across overview, full map and photo editor.
const topology = source as unknown as Topology
const { scale, translate } = topology.transform
const arcs: Position[][] = topology.arcs.map(arc => {
  let x = 0, y = 0
  return arc.map(([dx, dy]) => {
    x += dx; y += dy
    return [x * scale[0] + translate[0], y * scale[1] + translate[1]]
  })
})
function ring(indices: number[]): Position[] {
  const points: Position[] = []
  for (const index of indices) {
    const arc = index < 0 ? [...arcs[~index]!].reverse() : arcs[index]!
    if (points.length) points.pop()
    points.push(...arc)
  }
  return points
}
const land: FeatureCollection = {
  type: 'FeatureCollection',
  features: topology.objects.land.geometries.map(geometry => ({
    type: 'Feature',
    properties: {},
    geometry: (geometry.type === 'Polygon'
      ? { type: 'Polygon', coordinates: (geometry.arcs as number[][]).map(ring) }
      : { type: 'MultiPolygon', coordinates: (geometry.arcs as number[][][]).map(polygon => polygon.map(ring)) }
    ) as Geometry,
  })),
}

export function addOfflineMap(map: L.Map) {
  map.createPane('offline-land').style.zIndex = '150'
  const labels = map.createPane('offline-labels')
  labels.style.zIndex = '180'
  labels.style.pointerEvents = 'none'
  L.geoJSON(land, {
    pane: 'offline-land', interactive: false,
    style: { className: 'offline-land', weight: 1, fillOpacity: 1 },
    attribution: '<a href="https://www.naturalearthdata.com/">Natural Earth</a>',
  }).addTo(map)
  for (let lat = -60; lat <= 75; lat += 15) {
    L.polyline([[lat, -180], [lat, 180]], {
      pane: 'offline-land', className: 'offline-grid', weight: .5, interactive: false,
    }).addTo(map)
  }
  for (let lng = -180; lng <= 180; lng += 15) {
    L.polyline([[-80, lng], [85, lng]], {
      pane: 'offline-land', className: 'offline-grid', weight: .5, interactive: false,
    }).addTo(map)
  }
  // Geographic reference labels, deliberately distinct from photo heart markers.
  const cities: [string, number, number][] = [
    ['北京', 39.9, 116.4], ['上海', 31.23, 121.47], ['成都', 30.57, 104.06],
    ['西安', 34.34, 108.94], ['广州', 23.13, 113.26], ['昆明', 25.04, 102.71],
    ['乌鲁木齐', 43.82, 87.62], ['拉萨', 29.65, 91.14], ['哈尔滨', 45.8, 126.53],
    ['东京', 35.68, 139.69], ['曼谷', 13.76, 100.5], ['新加坡', 1.35, 103.82],
  ]
  for (const [name, lat, lng] of cities) {
    L.marker([lat, lng], {
      pane: 'offline-labels', interactive: false, keyboard: false,
      icon: L.divIcon({ className: 'offline-place', html: `<i></i><span>${name}</span>`, iconSize: [70, 20], iconAnchor: [3, 10] }),
    }).addTo(map)
  }
  let visible=true
  const updateLabels = () => { labels.style.opacity = visible && map.getZoom() <= 7 ? '1' : '0' }
  map.on('zoomend', updateLabels)
  updateLabels()
  return (next:boolean)=>{visible=next;map.getPane('offline-land')!.style.opacity=next?'1':'0';updateLabels()}
}

interface CountryGeometry extends TopologyGeometry { id?:string;properties?:Record<string,string> }
const countryObjects=(source as unknown as {objects:{countries:{geometries:CountryGeometry[]}}}).objects.countries.geometries
export const worldCountries:FeatureCollection={type:'FeatureCollection',features:countryObjects.map(geometry=>({
 type:'Feature',id:geometry.id,properties:geometry.properties||{},geometry:(geometry.type==='Polygon'?{type:'Polygon',coordinates:(geometry.arcs as number[][]).map(ring)}:{type:'MultiPolygon',coordinates:(geometry.arcs as number[][][]).map(polygon=>polygon.map(ring))}) as Geometry,
}))}
export function countryAt(latitude:number,longitude:number):string|null{
 const inRing=(points:Position[])=>{let inside=false;for(let i=0,j=points.length-1;i<points.length;j=i++){const a=points[i]!,b=points[j]!;if((a[1]!>latitude)!==(b[1]!>latitude)&&longitude<(b[0]!-a[0]!)*(latitude-a[1]!)/(b[1]!-a[1]!)+a[0]!)inside=!inside}return inside}
 for(const feature of worldCountries.features){const g=feature.geometry;if(g.type!=='Polygon'&&g.type!=='MultiPolygon')continue;const polygons=g.type==='Polygon'?[g.coordinates]:g.coordinates;for(const polygon of polygons){if(polygon[0]&&inRing(polygon[0])&&!polygon.slice(1).some(inRing))return String(feature.id)}}return null
}
