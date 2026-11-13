import type { NodeProps } from '@xyflow/react'
import { Minus, Pause, Play, Plus, RotateCcw } from 'lucide-react'
import { useEffect, useRef, useState } from 'react'
import type { BoardNode } from './board'
import { useCanvas } from './canvasContext'
import { Flash, Handles, Resizer } from './CanvasNodes'
import { ItemToolbar } from './CanvasToolbar'
import { clockFace, clockOf, timerChange, type TimerAct } from './timer'

function chime() {
  const audio = new AudioContext()
  for (const at of [0, 0.35]) {
    const tone = audio.createOscillator()
    const gain = audio.createGain()
    tone.frequency.value = 880
    gain.gain.setValueAtTime(0.2, audio.currentTime + at)
    gain.gain.exponentialRampToValueAtTime(0.001, audio.currentTime + at + 0.3)
    tone.connect(gain).connect(audio.destination)
    tone.start(audio.currentTime + at)
    tone.stop(audio.currentTime + at + 0.3)
  }
  setTimeout(() => void audio.close(), 1000)
}

// A countdown anyone at the board can start. It chimes when it runs out in
// front of the user.
export function TimerNode({ id, data, selected }: NodeProps<BoardNode>) {
  const { commit, connectSource } = useCanvas()
  const [now, setNow] = useState(Date.now)
  const clock = clockOf(data.item, now)
  const ran = useRef(false)
  useEffect(() => {
    if (ran.current && !clock.running && clock.left === 0) chime()
    ran.current = clock.running
    if (!clock.running) return
    const tick = setInterval(() => setNow(Date.now()), 250)
    return () => clearInterval(tick)
  }, [clock.running, clock.left])
  const act = (kind: TimerAct) => {
    setNow(Date.now())
    commit({ nodes: [{ id, ...timerChange(clockOf(data.item, Date.now()), kind, Date.now()) }] })
  }
  const done = clock.left === 0
  return (
    <>
      <Resizer item={data.item} selected={selected} minWidth={180} minHeight={100} />
      {selected ? <ItemToolbar label="Timer" item={data.item} /> : null}
      <Handles />
      <div className={`canvas-card canvas-timer${done ? ' done' : ''}${connectSource === id ? ' canvas-connect-source' : ''}`}>
        <div className="canvas-timer-face" role="timer" aria-label="Time left">{clockFace(clock.left)}</div>
        <div className="canvas-timer-controls nodrag">
          <button type="button" aria-label="One minute less" title="One minute less" onClick={() => act('shorter')}><Minus size={14} /></button>
          <button type="button" className="main" aria-label={clock.running ? 'Pause' : 'Start'} title={clock.running ? 'Pause' : 'Start'} onClick={() => act(clock.running ? 'pause' : 'start')}>
            {clock.running ? <Pause size={16} /> : <Play size={16} />}
          </button>
          <button type="button" aria-label="Reset" title="Reset" onClick={() => act('reset')}><RotateCcw size={14} /></button>
          <button type="button" aria-label="One minute more" title="One minute more" onClick={() => act('longer')}><Plus size={14} /></button>
        </div>
        <Flash id={id} />
      </div>
    </>
  )
}
