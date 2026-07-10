-- +goose Up
DELETE FROM integration_oauth_tokens
WHERE connection_id IN (
  SELECT 'mcp:' || id
  FROM mcp_servers
  WHERE lower(url) = 'https://mcp.deployink.com'
    OR lower(url) LIKE 'https://mcp.deployink.com/%'
    OR lower(url) = 'https://mcp.ml.ink'
    OR lower(url) LIKE 'https://mcp.ml.ink/%'
);

DELETE FROM mcp_servers
WHERE lower(url) = 'https://mcp.deployink.com'
  OR lower(url) LIKE 'https://mcp.deployink.com/%'
  OR lower(url) = 'https://mcp.ml.ink'
  OR lower(url) LIKE 'https://mcp.ml.ink/%';

-- +goose Down
SELECT 1;
