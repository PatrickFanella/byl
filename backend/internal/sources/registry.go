package sources

import "sort"

type entry struct {
	adapter Adapter
	info    Info
}

// Registry holds the configured adapters.
type Registry struct {
	byName map[string]entry
	client *Client
}

// NewRegistry builds every adapter against the shared client.
func NewRegistry(client *Client, localArchiveURLs []string) *Registry {
	r := &Registry{byName: map[string]entry{}, client: client}
	r.add(NewWikipedia(client, ""), Info{Description: "Wikipedia articles near a point: a place and a moment per article", SupportsNear: true})
	r.add(NewCommons(client, ""), Info{Description: "Wikimedia Commons images near a point, with licence metadata", SupportsNear: true})
	r.add(NewInternetArchive(client, ""), Info{Description: "Internet Archive texts, images, and films matching a query", SupportsQuery: true})
	r.add(NewLOC(client, ""), Info{Description: "Library of Congress items matching a query", SupportsQuery: true})
	r.add(NewURL(client, localArchiveURLs), Info{Description: "Configured local archive pages (LOCAL_ARCHIVE_URLS)", SupportsQuery: true})
	return r
}

func (r *Registry) add(a Adapter, info Info) {
	info.Name = a.Name()
	r.byName[a.Name()] = entry{adapter: a, info: info}
}

// Register adds or replaces an adapter; tests use it to inject fakes.
func (r *Registry) Register(a Adapter, info Info) { r.add(a, info) }

// Client returns the shared HTTP client, used to download media files.
func (r *Registry) Client() *Client { return r.client }

// Get returns an adapter by name.
func (r *Registry) Get(name string) (Adapter, bool) {
	e, ok := r.byName[name]
	return e.adapter, ok
}

// List returns adapter descriptions sorted by name.
func (r *Registry) List() []Info {
	out := make([]Info, 0, len(r.byName))
	for _, e := range r.byName {
		out = append(out, e.info)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
