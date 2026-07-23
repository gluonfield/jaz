package browsercontrol

import "strings"

const (
	browserTabLimit   = 100
	browserTabIDLimit = 512
)

func boundBrowserTabs(tabs []BrowserTab) []BrowserTab {
	out := make([]BrowserTab, 0, min(len(tabs), browserTabLimit))
	for _, tab := range tabs {
		if len(out) == browserTabLimit {
			break
		}
		id := strings.TrimSpace(tab.ID)
		if id == "" || len(id) > browserTabIDLimit {
			continue
		}
		tab.ID = id
		tab.Title = shortenText(tab.Title, 500)
		tab.URL = truncateUTF8(strings.TrimSpace(tab.URL), 2000)
		tab.Ownership = shortenText(tab.Ownership, 100)
		out = append(out, tab)
	}
	return out
}
