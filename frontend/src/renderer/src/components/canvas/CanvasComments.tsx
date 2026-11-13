import { ViewportPortal } from '@xyflow/react'
import { MessageSquare } from 'lucide-react'
import { useState } from 'react'
import type { CanvasItem } from '@/lib/api/canvas'
import { rectOfNode, type BoardNode } from './board'
import { useCanvas } from './canvasContext'
import { Menu } from './CanvasMenu'

export interface Comment {
  by: 'user' | 'agent'
  text: string
}

export function commentsOf(item: CanvasItem): Comment[] {
  const comments = Array.isArray(item.comments) ? item.comments : []
  return comments.flatMap((comment) => (typeof comment?.text === 'string' && comment.text ? [{ by: comment.by === 'agent' ? 'agent' as const : 'user' as const, text: comment.text }] : []))
}

// How many comments each item carries, at its top right corner.
export function CommentBadges({ nodes }: { nodes: BoardNode[] }) {
  return (
    <ViewportPortal>
      {nodes.map((node) => {
        const count = commentsOf(node.data.item).length
        return count ? <div key={node.id} className="canvas-comment-badge" title="Comments" style={{ left: node.position.x + rectOfNode(node).width, top: node.position.y }}>{count}</div> : null
      })}
    </ViewportPortal>
  )
}

// Notes pinned to an item, between the user and the agent.
export function Comments({ item }: { item: CanvasItem }) {
  const { commit } = useCanvas()
  const [draft, setDraft] = useState('')
  const comments = commentsOf(item)
  const add = () => {
    if (!draft.trim()) return
    commit({ nodes: [{ id: item.id, comments: [...comments, { by: 'user', text: draft.trim() }] }] })
    setDraft('')
  }
  return (
    <Menu label="Comments" stay face={<><MessageSquare size={16} />{comments.length ? <span className="canvas-toolbar-count">{comments.length}</span> : null}</>}>
      <div className="canvas-comments">
        {comments.map((comment, index) => (
          <p key={index} className={comment.by}><b>{comment.by === 'agent' ? 'Jaz' : 'You'}</b>{comment.text}</p>
        ))}
        <input
          value={draft}
          placeholder="Comment on this item"
          aria-label="Comment on this item"
          onMouseDown={(event) => event.stopPropagation()}
          onChange={(event) => setDraft(event.target.value)}
          onKeyDown={(event) => {
            if (event.key === 'Enter') add()
          }}
        />
        {comments.length ? <button type="button" className="canvas-toolbar-row" onClick={() => commit({ nodes: [{ id: item.id, comments: null }] })}>Resolve</button> : null}
      </div>
    </Menu>
  )
}
