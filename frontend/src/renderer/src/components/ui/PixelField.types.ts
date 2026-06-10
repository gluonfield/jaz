export type PixelFieldShapeName = string

export type PixelFieldShapeFrame = {
  shape: PixelFieldShapeName
  cx: number
  cy: number
  scale: number
}

export type PixelFieldShapeChoiceContext = {
  playlist: readonly PixelFieldShapeName[]
  lastShape: PixelFieldShapeName | null
  constructionCount: number
  defaultShape: () => PixelFieldShapeName | null
}

export type PixelFieldActiveShapeContext = {
  shape: PixelFieldShapeName
  frame: PixelFieldShapeFrame
}

export type PixelFieldActiveShapeState = {
  /** Keep the current construction on screen; the field applies a short grace after this clears. */
  hold?: boolean
  /** 0-1 swell applied to the active construction. */
  emphasis?: boolean | number
  holdGraceSeconds?: number
}

export type PixelFieldLifecycle = {
  chooseNextShape?: (context: PixelFieldShapeChoiceContext) => PixelFieldShapeName | null | undefined
  activeShape?: (context: PixelFieldActiveShapeContext) => PixelFieldActiveShapeState | null | undefined
}
