import test from 'node:test'
import assert from 'node:assert/strict'
import {toAmap,fromAmap} from '../src/mapCoordinates.ts'

test('GPS reference point aligns with the Amap display coordinate',()=>{
 const [lat,lon]=toAmap([39.908823,116.39747])
 assert.ok(Math.abs(lat-39.910226)<0.00001)
 assert.ok(Math.abs(lon-116.403714)<0.00001)
})
test('switching maps or picking a point does not accumulate coordinate drift',()=>{
 for(const point of [[39.908823,116.39747],[30.25,120.15],[23.13,113.26],[34.34,108.94]]){
  let current=[...point]
  for(let i=0;i<20;i++)current=fromAmap(toAmap(current))
  assert.ok(Math.abs(current[0]-point[0])<1e-7)
  assert.ok(Math.abs(current[1]-point[1])<1e-7)
 }
})
test('overseas coordinates remain unshifted and inputs are not mutated',()=>{
 for(const point of [[51.5,-.12],[-33.86,151.21],[35.68,139.69],[1.35,103.82],[37.57,126.98],[34.69,135.5],[25.03,121.56]]){
  const before=[...point]
  assert.deepEqual(toAmap(point),before)
  assert.deepEqual(fromAmap(point),before)
  assert.deepEqual(point,before)
 }
})
