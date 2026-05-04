package fetcher

type NetworkEntry struct {
	URL          string `json:"url"`
	Method       string `json:"method"`
	Status       int    `json:"status"`
	ResourceType string `json:"resource_type"`
}

type JSFile struct {
	URL     string `json:"url"`
	Content string `json:"content"`
}

type FormField struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

type Form struct {
	Action string      `json:"action"`
	Method string      `json:"method"`
	Fields []FormField `json:"fields"`
}

// FetchResult is the parsed output from the fetcher container.
// Screenshot is a base64-encoded PNG as emitted by the container JSON.
type FetchResult struct {
	FinalURL    string         `json:"final_url"`
	RenderedDOM string         `json:"rendered_dom"`
	Screenshot  string         `json:"screenshot"`
	NetworkLog  []NetworkEntry `json:"network_log"`
	JSFiles     []JSFile       `json:"js_files"`
	Forms       []Form         `json:"forms"`
}
