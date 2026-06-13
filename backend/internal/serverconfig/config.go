package serverconfig

import "strings"

const (
	JazToolsMCPPath       = "/mcp/jaztools"
	JazToolsMCPCompatPath = "/mcp/jaz"
	JazmemMCPPath         = "/mcp/jazmem"
)

type Config struct {
	Addr string
}

type URLs struct {
	JazToolsMCP string
	JazmemMCP   string
}

func New(addr string) Config {
	return Config{Addr: strings.TrimSpace(addr)}
}

func NewURLs(config Config) URLs {
	return URLs{
		JazToolsMCP: localURL(config.Addr, JazToolsMCPPath),
		JazmemMCP:   localURL(config.Addr, JazmemMCPPath),
	}
}

func localURL(addr, path string) string {
	host := strings.TrimSpace(addr)
	if strings.HasPrefix(host, ":") {
		host = "127.0.0.1" + host
	}
	return "http://" + host + path
}
