import { expect, test } from 'bun:test'
import { appleMapsURL, googleMapsURL, parseMapFence, parsePlaceMap } from './maps'

test('a map keeps only places it can pin and links it can safely open', () => {
  const map = parsePlaceMap({
    title: '  ',
    places: [
      { name: 'Caravan', latitude: 51.5357, longitude: -0.1253, rating: 4.4, reviews: 3100.4, url: 'javascript:alert(1)', address: ' 1 Granary Square ' },
      { name: 'No coordinates', address: 'London' },
      { name: 'Off the earth', latitude: 95, longitude: 0 },
      { name: ' ', latitude: 1, longitude: 1 },
      { name: 'Origin', latitude: 51.5313, longitude: -0.1233, rating: 9, url: 'https://origincoffee.co.uk' },
    ],
  })
  expect(map).toEqual({
    title: 'Map',
    places: [
      { name: 'Caravan', latitude: 51.5357, longitude: -0.1253, address: '1 Granary Square', category: undefined, note: undefined, rating: 4.4, reviews: 3100, price: undefined, url: undefined },
      { name: 'Origin', latitude: 51.5313, longitude: -0.1233, address: undefined, category: undefined, note: undefined, rating: undefined, reviews: undefined, price: undefined, url: 'https://origincoffee.co.uk' },
    ],
  })
  expect(parsePlaceMap({ title: 'Empty', places: [{ name: 'Nowhere' }] })).toBeUndefined()
})

test('a jaz-map fence renders only when its JSON is complete', () => {
  const fence = '{"title":"Coffee","places":[{"name":"Caravan","latitude":51.5357,"longitude":-0.1253}]}'
  expect(parseMapFence(fence)?.places[0].name).toBe('Caravan')
  expect(parseMapFence(fence.slice(0, -4))).toBeUndefined()
})

test('Google Maps and Apple Maps open the listing by name and address, or the exact point without one', () => {
  const listed = { name: 'Caravan', latitude: 51.5357, longitude: -0.1253, address: '1 Granary Square, London N1C 4AA' }
  expect(googleMapsURL(listed)).toBe('https://www.google.com/maps/search/?api=1&query=Caravan%2C+1+Granary+Square%2C+London+N1C+4AA')
  expect(appleMapsURL(listed)).toBe('https://maps.apple.com/?q=Caravan%2C+1+Granary+Square%2C+London+N1C+4AA&sll=51.5357%2C-0.1253')
  const point = { name: 'Trailhead', latitude: 46.5586, longitude: 7.8358 }
  expect(googleMapsURL(point)).toBe('https://www.google.com/maps/search/?api=1&query=46.5586%2C7.8358')
  expect(appleMapsURL(point)).toBe('https://maps.apple.com/?q=Trailhead&ll=46.5586%2C7.8358')
})
