package sources

import "context"

// Placeholders so the registry compiles; replaced by their own files in later tasks.

type InternetArchive struct {
	client *Client
	base   string
}

func NewInternetArchive(client *Client, base string) *InternetArchive {
	return &InternetArchive{client: client, base: base}
}
func (a *InternetArchive) Name() string                                       { return "internet_archive" }
func (a *InternetArchive) Discover(context.Context, Request) ([]Found, error) { return nil, nil }

type LOC struct {
	client *Client
	base   string
}

func NewLOC(client *Client, base string) *LOC                     { return &LOC{client: client, base: base} }
func (a *LOC) Name() string                                       { return "loc" }
func (a *LOC) Discover(context.Context, Request) ([]Found, error) { return nil, nil }

type URL struct {
	client *Client
	urls   []string
}

func NewURL(client *Client, urls []string) *URL                   { return &URL{client: client, urls: urls} }
func (a *URL) Name() string                                       { return "url" }
func (a *URL) Discover(context.Context, Request) ([]Found, error) { return nil, nil }
