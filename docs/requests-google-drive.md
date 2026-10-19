# Google Drive

Request: replace the Google-hosted preview implementation with working Drive access, preserving the existing tools.

- [completed] Serve all eight tools through the standard Drive API, with the same account namespaces and existing inputs/outputs.
- [completed] Reuse saved OAuth grants and refresh; support personal accounts and Workspace/shared drives.
- [completed] Add a server-side file input to create_file for streaming uploads without inline base64.
- [completed] Verify the real MCP transport, Google API boundaries and existing-account upgrade; run the full suite and strict review.

Contract source: Google's live tools/list, captured in ignored runs/drive-api/google-tool-contracts.json on 2026-10-19.

Implementation: eight account-scoped MCP tools run in process and call Drive API v3. Saved connections and grants need no migration or new consent. Existing query aliases are translated to Drive API queries. create_file and download_file_content additionally accept an absolute server-side filePath for streaming.

Verification: real-account calls passed for all eight tools, native document creation/readback, and a 3,300,040-byte binary upload/download with matching hashes. All three temporary files were deleted. Full backend suite/vet and 339 frontend tests/typecheck/lint pass; account upgrade/isolation, refresh/disconnect, shared drives, exports/revisions, uploads and eight negative controls pass. No live model calls.

Limits: snippets use file descriptions. read_file_content exports native Google documents and reads UTF-8 text; Sheets text exports contain only the first worksheet and say so in the result. Sheets downloads default to XLSX, preserving every worksheet. Binary formats report textFormattingNotSupported and remain available through download_file_content, including direct downloads to disk. Inline downloads are limited to 32 MiB; streaming downloads avoid that limit. No external extraction service or extra dependency was added.

Strict follow-up review:
- [completed] Use resumable upload sessions for every content source; remove multipart envelope assembly and share HTTP error handling with revision exports.
- [completed] Make first-worksheet-only text exports explicit and default spreadsheet downloads to the complete XLSX workbook.
- [completed] Verify large and empty uploads, rejected upload locations, failed-download cleanup and the full backend suite.

Live follow-up verification: all eight tools, a 6,600,040-byte hash-checked round trip, and default XLSX download preserving a two-worksheet workbook pass. Source MIME type and length are recorded when starting uploads, including native conversion. Eight temporary files across review probes were deleted and separately confirmed absent; no named verification files remain. Final 103-package backend suite, vet, build, targeted race checks and six compiled behavioral controls pass. No remaining structural blocker; no new dependency, service or credential store.

Provider contracts: [upload methods](https://developers.google.com/workspace/drive/api/guides/manage-uploads) and [export formats](https://developers.google.com/workspace/drive/api/guides/ref-export-formats). CSV/TSV spreadsheet exports cover only the first worksheet.
