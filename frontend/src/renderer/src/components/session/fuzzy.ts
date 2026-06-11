// Small fuzzy subsequence matcher for the composer's @/$ popups — the same
// scoring family as the nucleo matcher behind Codex's file search:
// case-insensitive subsequence, bonuses for path/word boundaries and
// consecutive runs, penalties for gaps. A query that names a directory
// scores the directory and its children identically up to the tie-breaks, so
// "folder first, then its files" falls out of the ranking naturally.

export interface FuzzyMatch {
  score: number
  /** candidate indices of the matched characters, ascending */
  indices: number[]
}

const SEGMENT_BONUS = 8 // start of the candidate or after '/'
const WORD_BONUS = 4 // after '-', '_', '.', ' ' or at a camelCase hump
const CONSECUTIVE_BONUS = 6
const GAP_PENALTY = 0.5 // per skipped char, capped per gap
const GAP_PENALTY_CAP = 4

// fuzzyMatch scores query against candidate, or returns null when query is
// not a subsequence. An empty query matches everything with score 0 so
// callers can fall back to their natural ordering.
export function fuzzyMatch(query: string, candidate: string): FuzzyMatch | null {
  if (query === '') return { score: 0, indices: [] }
  const q = query.toLowerCase()
  const c = candidate.toLowerCase()
  if (q.length > c.length) return null

  // Greedy from a single start point can miss a better alignment further
  // right ("ab" in "axb/ab"); retry from each occurrence of the first query
  // char (bounded) and keep the best-scoring alignment.
  let best: FuzzyMatch | null = null
  let from = c.indexOf(q[0])
  for (let starts = 0; from !== -1 && starts < 8; starts++, from = c.indexOf(q[0], from + 1)) {
    const match = greedyFrom(q, c, candidate, from)
    if (match && (!best || match.score > best.score)) best = match
  }
  return best
}

function greedyFrom(
  q: string,
  c: string,
  candidate: string,
  start: number,
): FuzzyMatch | null {
  const indices: number[] = [start]
  let at = start + 1
  for (let qi = 1; qi < q.length; qi++) {
    const idx = c.indexOf(q[qi], at)
    if (idx === -1) return null
    indices.push(idx)
    at = idx + 1
  }

  let score = 0
  for (let k = 0; k < indices.length; k++) {
    const i = indices[k]
    const prev = candidate[i - 1]
    if (i === 0 || prev === '/') score += SEGMENT_BONUS
    else if (
      prev === '-' ||
      prev === '_' ||
      prev === '.' ||
      prev === ' ' ||
      (/[a-z0-9]/.test(prev) && /[A-Z]/.test(candidate[i]))
    ) {
      score += WORD_BONUS
    }
    if (k > 0) {
      const gap = i - indices[k - 1] - 1
      if (gap === 0) score += CONSECUTIVE_BONUS
      else score -= Math.min(gap * GAP_PENALTY, GAP_PENALTY_CAP)
    }
    score += 1
  }
  return { score, indices }
}
