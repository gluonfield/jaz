import { createPiece, PieceAuth } from '@activepieces/pieces-framework';
import { storeToLake } from './lib/actions/store-to-lake';

export const lakeWriter = createPiece({
  displayName: 'Lake Writer',
  description:
    'Store incoming items as markdown files in the local data lake (or jazmem inbox) with an append-only journal for incremental agent triage.',
  auth: PieceAuth.None(),
  minimumSupportedRelease: '0.36.1',
  logoUrl: 'https://cdn.activepieces.com/pieces/markdown.svg',
  authors: ['wins'],
  actions: [storeToLake],
  triggers: [],
});
