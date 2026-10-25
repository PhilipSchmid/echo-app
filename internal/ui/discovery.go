package ui

import (
	"encoding/xml"
	"strings"
)

type pageMetadata struct {
	Path        string
	Title       string
	Description string
	Canonical   string
}

var discoverablePages = []pageMetadata{
	{Path: "/ui/", Title: "Echo Observatory — HTTP, TLS & network debugging", Description: "Inspect your request, IP address, headers and TLS connection. Test HTTP, WebSocket, gRPC and HTTP/3 with an open-source network debugging tool."},
	{Path: "/ui/workbench/", Title: "Request workbench — Echo Observatory", Description: "Send HTTP requests, inspect responses and test WebSocket echo in your browser with Echo Observatory."},
	{Path: "/ui/examples/", Title: "HTTP & WebSocket examples — Echo Observatory", Description: "Try HTTP requests, response controls and WebSocket echo with practical, ready-to-run Echo Observatory examples."},
	{Path: "/ui/connect/", Title: "curl, wget & protocol examples — Echo Observatory", Description: "Connect with curl, wget, websocat and grpcurl. Explore HTTP, HTTPS, HTTP/2, HTTP/3, WebSocket, TCP and gRPC examples."},
	{Path: "/ui/privacy/", Title: "Privacy & data — Echo Observatory", Description: "How Echo Observatory handles requests, logs, credential redaction and browser preferences."},
}

func metadataFor(path, publicURL string) pageMetadata {
	for _, page := range discoverablePages {
		if page.Path == path {
			if publicURL != "" {
				page.Canonical = strings.TrimSuffix(publicURL, "/") + path
			}
			return page
		}
	}
	return pageMetadata{}
}

func sitemapDocument(publicURL string) ([]byte, error) {
	type entry struct {
		Location string `xml:"loc"`
	}
	document := struct {
		XMLName xml.Name `xml:"urlset"`
		XMLNS   string   `xml:"xmlns,attr"`
		URLs    []entry  `xml:"url"`
	}{XMLNS: "http://www.sitemaps.org/schemas/sitemap/0.9"}
	for _, page := range discoverablePages {
		document.URLs = append(document.URLs, entry{metadataFor(page.Path, publicURL).Canonical})
	}
	data, err := xml.MarshalIndent(document, "", "  ")
	if err != nil {
		return nil, err
	}
	return append([]byte(xml.Header), data...), nil
}
