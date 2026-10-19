# Google Drive

Request: replace the Google-hosted preview implementation with working Drive access, preserving the existing tools.

- [completed] Serve all eight tools through the standard Drive API, with the same account namespaces and existing inputs/outputs.
- [completed] Reuse saved OAuth grants and refresh; support personal accounts and Workspace/shared drives.
- [completed] Add a server-side file input to create_file for streaming uploads without inline base64.
- [completed] Verify the real MCP transport, Google API boundaries and existing-account upgrade; run the full suite and strict review.

Contract source: Google's live tools/list, captured in ignored runs/drive-api/google-tool-contracts.json on 2026-10-19.

Implementation: eight account-scoped MCP tools run in process and call Drive API v3. Saved connections and grants need no migration or new consent. Existing query aliases are translated to Drive API queries. create_file and download_file_content additionally accept an absolute server-side filePath for streaming.

Verification: real-account calls passed for all eight tools, native document creation/readback, and a 3,300,040-byte binary upload/download with matching hashes. All three temporary files were deleted. Full backend suite/vet and 339 frontend tests/typecheck/lint pass; account upgrade/isolation, refresh/disconnect, shared drives, exports/revisions, uploads and eight negative controls pass. No live model calls.

Limits: snippets use file descriptions. read_file_content exports native Google documents and reads UTF-8 text; binary formats report textFormattingNotSupported and remain available through download_file_content, including direct downloads to disk. Inline downloads are limited to 32 MiB; streaming downloads avoid that limit. No external extraction service or extra dependency was added.
