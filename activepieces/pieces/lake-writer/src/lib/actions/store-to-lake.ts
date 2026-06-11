import { createAction, Property } from '@activepieces/pieces-framework';
import fs from 'node:fs';
import path from 'node:path';
import crypto from 'node:crypto';

const JOURNAL_DIR = '/data/.journal';

export const storeToLake = createAction({
  name: 'store_to_lake',
  displayName: 'Store to Lake',
  description:
    'Write one item as a markdown file with provenance frontmatter. Idempotent: the file name derives from the external id, so replays are no-ops. Every write is journaled for incremental triage.',
  props: {
    source: Property.ShortText({
      displayName: 'Source',
      description: 'Connector lane, e.g. gmail, slack, linear, calendar, linkedin',
      required: true,
    }),
    externalId: Property.ShortText({
      displayName: 'External ID',
      description: 'Stable upstream id (message id, event id). Drives idempotent naming.',
      required: true,
    }),
    occurredAt: Property.ShortText({
      displayName: 'Occurred At',
      description: 'ISO-8601 timestamp of the event itself',
      required: true,
    }),
    title: Property.ShortText({ displayName: 'Title', required: true }),
    body: Property.LongText({ displayName: 'Body', required: true }),
    url: Property.ShortText({ displayName: 'URL', required: false }),
    people: Property.Array({
      displayName: 'People',
      description: 'Emails or names involved',
      required: false,
    }),
    destination: Property.StaticDropdown({
      displayName: 'Destination',
      description: 'Lake for raw archive; jazmem inbox only for explicit "remember this" captures',
      required: false,
      defaultValue: '/data',
      options: {
        disabled: false,
        options: [
          { label: 'Data lake (/data)', value: '/data' },
          { label: 'jazmem inbox (/memory-inbox)', value: '/memory-inbox' },
        ],
      },
    }),
  },
  async run(context) {
    const p = context.propsValue;
    const destination = p.destination ?? '/data';
    const hash = crypto
      .createHash('sha256')
      .update(p.externalId || p.body)
      .digest('hex')
      .slice(0, 8);
    const day = /^\d{4}-\d{2}-\d{2}/.test(p.occurredAt)
      ? p.occurredAt.slice(0, 10)
      : new Date().toISOString().slice(0, 10);
    const slug =
      p.title
        .toLowerCase()
        .replace(/[^a-z0-9]+/g, '-')
        .replace(/(^-|-$)/g, '')
        .slice(0, 48) || 'untitled';
    const fileName = `${day}-${slug}-${hash}.md`;
    const dir =
      destination === '/memory-inbox'
        ? '/memory-inbox'
        : path.join('/data', p.source, day.slice(0, 4), day.slice(5, 7));
    const file = path.join(dir, fileName);
    if (fs.existsSync(file)) {
      return { path: file, skipped: true };
    }

    const people = (p.people ?? []).map((value) => String(value));
    const frontmatter = [
      '---',
      `title: ${JSON.stringify(p.title)}`,
      `type: ${destination === '/memory-inbox' ? 'inbox' : p.source}`,
      `source: ${p.source}`,
      `external_id: ${JSON.stringify(p.externalId)}`,
      `occurred_at: ${p.occurredAt}`,
      p.url ? `url: ${JSON.stringify(p.url)}` : null,
      people.length > 0 ? `people: ${JSON.stringify(people)}` : null,
      `fetched_at: ${new Date().toISOString()}`,
      '---',
    ]
      .filter(Boolean)
      .join('\n');

    fs.mkdirSync(dir, { recursive: true });
    fs.writeFileSync(file, `${frontmatter}\n\n# ${p.title}\n\n${p.body}\n`);

    // Journal keyed by write date, not occurred_at, so the triage cursor is monotonic.
    const today = new Date().toISOString().slice(0, 10);
    fs.mkdirSync(JOURNAL_DIR, { recursive: true });
    fs.appendFileSync(
      path.join(JOURNAL_DIR, `${today}.jsonl`),
      JSON.stringify({
        path: file,
        source: p.source,
        destination,
        occurred_at: p.occurredAt,
        title: p.title,
      }) + '\n'
    );
    return { path: file, skipped: false };
  },
});
