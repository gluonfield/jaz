const GESTURE_GAP_MS = 120
const STEP_TRAVEL = 40

// Turns a stream of horizontal wheel deltas into one step per touchpad swipe.
// The touchpad keeps sending coasting deltas for about a second after the fingers lift,
// so a new swipe is recognised by the deltas growing again after they had been shrinking.
export function createSwipe(): (deltaX: number, time: number) => -1 | 0 | 1 {
  let at = 0
  let travel = 0
  let done = false
  let peak = 0
  let low = Infinity
  return (deltaX, time) => {
    const size = Math.abs(deltaX)
    if (done) {
      peak = Math.max(peak, size)
      if (size < peak * 0.7) {
        low = Math.min(low, size)
      }
    }
    if (time - at > GESTURE_GAP_MS || Math.sign(deltaX) !== Math.sign(travel) || size > low * 2 + 4) {
      travel = 0
      done = false
      peak = 0
      low = Infinity
    }
    at = time
    travel += deltaX
    if (done || Math.abs(travel) < STEP_TRAVEL) {
      return 0
    }
    done = true
    return Math.sign(travel) as -1 | 1
  }
}
