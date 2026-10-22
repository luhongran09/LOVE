import { usesAmapOffset } from './chinaBoundary.ts'
// Photo EXIF, Nominatim and stored coordinates use WGS84. Only the Amap
// display uses GCJ-02; never persist the shifted display coordinates.
export type Coordinates = [number, number]
const PI = Math.PI, A = 6378245, EE = 0.006693421622965943
function outside([lat, lon]: Coordinates) { return !usesAmapOffset(lat,lon) }
function offset([lat, lon]: Coordinates): Coordinates {
 const x=lon-105,y=lat-35
 let dLat=-100+2*x+3*y+.2*y*y+.1*x*y+.2*Math.sqrt(Math.abs(x))
 let dLon=300+x+2*y+.1*x*x+.1*x*y+.1*Math.sqrt(Math.abs(x))
 dLat+=(20*Math.sin(6*x*PI)+20*Math.sin(2*x*PI))*2/3
 dLat+=(20*Math.sin(y*PI)+40*Math.sin(y/3*PI))*2/3
 dLat+=(160*Math.sin(y/12*PI)+320*Math.sin(y*PI/30))*2/3
 dLon+=(20*Math.sin(6*x*PI)+20*Math.sin(2*x*PI))*2/3
 dLon+=(20*Math.sin(x*PI)+40*Math.sin(x/3*PI))*2/3
 dLon+=(150*Math.sin(x/12*PI)+300*Math.sin(x/30*PI))*2/3
 const rad=lat/180*PI,sin=Math.sin(rad),magic=1-EE*sin*sin,sqrt=Math.sqrt(magic)
 return [dLat*180/((A*(1-EE))/(magic*sqrt)*PI),dLon*180/(A/sqrt*Math.cos(rad)*PI)]
}
export function toAmap(coords:Coordinates):Coordinates {
 if(outside(coords))return [...coords]
 const delta=offset(coords);return [coords[0]+delta[0],coords[1]+delta[1]]
}
export function fromAmap(coords:Coordinates):Coordinates {
 if(outside(coords))return [...coords]
 let result:Coordinates=[...coords]
 for(let i=0;i<6;i++){const projected=toAmap(result);result=[result[0]+coords[0]-projected[0],result[1]+coords[1]-projected[1]]}
 return result
}
