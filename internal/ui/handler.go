package ui

import (
	"bytes"
	"embed"
	"encoding/json"
	"html/template"
	"net/http"
	"strconv"
	"strings"

	"github.com/PhilipSchmid/echo-observatory/v2/internal/config"
	"github.com/PhilipSchmid/echo-observatory/v2/internal/handlers"
)

//go:embed assets/*
var assets embed.FS

var pageTemplates = template.Must(template.ParseFS(assets, "assets/*.html"))

type pageView struct {
	Page         pageMetadata
	Version      string
	Request      handlers.HTTPResponse
	Stack        stackDetails
	JSON         string
	Headers      bool
	Unsafe       bool
	Certificates bool
}

type renderedPage struct {
	data []byte
	err  error
}

func renderPage(filename string, view pageView) renderedPage {
	var rendered bytes.Buffer
	err := pageTemplates.ExecuteTemplate(&rendered, strings.TrimPrefix(filename, "assets/"), view)
	return renderedPage{data: rendered.Bytes(), err: err}
}

// NewHandler reflects page requests and serves the embedded interface and public settings.
func NewHandler(cfg *config.Config, listener string, sources ...handlers.CertificateSource) http.Handler {
	buildVersion := applicationVersion()
	var certificateSource handlers.CertificateSource
	if len(sources) > 0 {
		certificateSource = sources[0]
	}
	sitemap, sitemapErr := sitemapDocument(cfg.PublicURL)
	files := staticAssets()
	pages := make(map[string]renderedPage)
	for _, page := range discoverablePages {
		if page.Path == "/ui/" {
			continue
		}
		filename := strings.TrimSuffix(strings.TrimPrefix(page.Path, "/ui/"), "/") + ".html"
		pages[page.Path] = renderPage(filename, pageView{
			Page: metadataFor(page.Path, cfg.PublicURL), Version: buildVersion, Headers: cfg.PrintHeaders, Unsafe: cfg.UnsafeHeaders, Certificates: cfg.TLS || cfg.QUIC || cfg.GRPCTLS,
		})
	}
	settings, _ := json.Marshal(struct {
		H2C              bool   `json:"h2c"`
		TLS              bool   `json:"tls"`
		TCP              bool   `json:"tcp"`
		GRPC             bool   `json:"grpc"`
		GRPCTLS          bool   `json:"grpc_tls"`
		QUIC             bool   `json:"quic"`
		HTTP3IPv4Only    bool   `json:"http3_ipv4_only,omitempty"`
		HTTPPort         string `json:"http_port"`
		TLSPort          string `json:"tls_port"`
		TCPPort          string `json:"tcp_port"`
		GRPCPort         string `json:"grpc_port"`
		QUICPort         string `json:"quic_port"`
		UnsafeHeaders    bool   `json:"unsafe_headers"`
		PrintHeaders     bool   `json:"print_headers"`
		ResponseControls bool   `json:"response_controls"`
		DemoCertificate  bool   `json:"demo_certificate"`
		MaxRequestSize   int64  `json:"max_request_size"`
		MaxResponseSize  int64  `json:"max_response_size"`
	}{
		H2C: cfg.H2C, TLS: cfg.TLS, TCP: cfg.TCP, GRPC: cfg.GRPC, GRPCTLS: cfg.GRPCTLS,
		QUIC: cfg.QUIC, HTTPPort: advertisedPort(cfg.HTTPAdvertisePort, cfg.HTTPPort), TLSPort: advertisedPort(cfg.TLSAdvertisePort, cfg.TLSPort),
		HTTP3IPv4Only: cfg.QUIC && cfg.UIHTTP3IPv4Only,
		TCPPort:       advertisedPort(cfg.TCPAdvertisePort, cfg.TCPPort), GRPCPort: advertisedPort(cfg.GRPCAdvertisePort, cfg.GRPCPort), QUICPort: advertisedPort(cfg.QUICAdvertisePort, cfg.QUICPort),
		PrintHeaders: cfg.PrintHeaders, UnsafeHeaders: cfg.UnsafeHeaders, ResponseControls: cfg.ResponseControls,
		DemoCertificate: cfg.TLSCertFile == "" && cfg.ACMEDomain == "", MaxRequestSize: cfg.MaxRequestSize,
		MaxResponseSize: cfg.MaxResponseSize,
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Robots-Tag", "noindex")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src 'self'; connect-src 'self'; img-src 'self'; font-src 'self'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'")

		var filename, contentType, location string
		switch r.URL.Path {
		case "/robots.txt":
			contentType = "text/plain; charset=utf-8"
		case "/sitemap.xml":
			if cfg.PublicURL == "" {
				http.NotFound(w, r)
				return
			}
			contentType = "application/xml; charset=utf-8"
		case "/ui":
			location = "ui/"
		case "/ui/examples":
			location = "examples/"
		case "/ui/workbench":
			location = "workbench/"
		case "/ui/connect":
			location = "connect/"
		case "/ui/privacy":
			location = "privacy/"
		case "/ui/":
			filename, contentType = "assets/index.html", "text/html; charset=utf-8"
		case "/ui/examples/":
			filename, contentType = "assets/examples.html", "text/html; charset=utf-8"
		case "/ui/workbench/":
			filename, contentType = "assets/workbench.html", "text/html; charset=utf-8"
		case "/ui/connect/":
			filename, contentType = "assets/connect.html", "text/html; charset=utf-8"
		case "/ui/privacy/":
			filename, contentType = "assets/privacy.html", "text/html; charset=utf-8"
		case "/ui/app.css":
			filename, contentType = "assets/app.css", "text/css; charset=utf-8"
		case "/ui/app.js", "/ui/visit.js", "/ui/common.js", "/ui/presets.js", "/ui/guide.js", "/ui/controls.js", "/ui/highlight.js", "/ui/terminal.js", "/ui/privacy.js", "/ui/headers.js", "/ui/certificate.js":
			filename, contentType = "assets/"+r.URL.Path[len("/ui/"):], "text/javascript; charset=utf-8"
		case "/ui/logo.svg", "/ui/favicon.svg":
			filename, contentType = "assets/"+r.URL.Path[len("/ui/"):], "image/svg+xml"
		case "/ui/nunito-latin.woff2", "/ui/sometype-mono-latin.woff2":
			filename, contentType = "assets/"+r.URL.Path[len("/ui/"):], "font/woff2"
		case "/ui/config.json", "/ui/certificate.json":
			contentType = "application/json"
		default:
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if location != "" {
			if r.URL.RawQuery != "" {
				location += "?" + r.URL.RawQuery
			}
			w.Header().Set("Location", location)
			w.WriteHeader(http.StatusMovedPermanently)
			return
		}
		data := settings
		page := metadataFor(r.URL.Path, cfg.PublicURL)
		if page.Path != "" {
			rendered := pages[r.URL.Path]
			if filename == "assets/index.html" {
				view := pageView{Page: page, Version: buildVersion, Headers: cfg.PrintHeaders, Unsafe: cfg.UnsafeHeaders, Certificates: cfg.TLS || cfg.QUIC || cfg.GRPCTLS}
				request := handlers.NewHTTPResponse(r, cfg, listener)
				// HEAD describes the corresponding GET representation without sending it.
				if r.Method == http.MethodHead {
					request.HTTPMethod = http.MethodGet
				}
				requestJSON, err := json.MarshalIndent(request, "", "  ")
				if err != nil {
					http.Error(w, "Unable to describe request", http.StatusInternalServerError)
					return
				}
				view.Request, view.Stack, view.JSON = request, newStackDetails(request), string(requestJSON)
				rendered = renderPage(filename, view)
			}
			if rendered.err != nil {
				http.Error(w, "Unable to render request", http.StatusInternalServerError)
				return
			}
			data = rendered.data
			if cfg.PublicURL != "" && r.URL.RawQuery == "" && !r.URL.ForceQuery {
				w.Header().Del("X-Robots-Tag")
			}
		} else if r.URL.Path == "/ui/certificate.json" {
			mode := certificateMode(cfg)
			var certificate *handlers.ServerCertificate
			if mode != "disabled" && certificateSource != nil {
				certificate = certificateSource()
			}
			var err error
			data, err = json.Marshal(struct {
				Mode        string                      `json:"mode"`
				Certificate *handlers.ServerCertificate `json:"certificate"`
			}{Mode: mode, Certificate: certificate})
			if err != nil {
				http.Error(w, "Unable to describe server certificate", http.StatusInternalServerError)
				return
			}
		} else if r.URL.Path == "/robots.txt" {
			data = []byte("User-agent: *\nDisallow: /\n")
			if cfg.PublicURL != "" {
				data = []byte("User-agent: *\nAllow: /\n\nSitemap: " + strings.TrimSuffix(cfg.PublicURL, "/") + "/sitemap.xml\n")
			}
		} else if r.URL.Path == "/sitemap.xml" {
			if sitemapErr != nil {
				http.Error(w, "Unable to render sitemap", http.StatusInternalServerError)
				return
			}
			data = sitemap
		} else if filename != "" {
			asset, ok := files[filename]
			if !ok {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", contentType)
			asset.serve(w, r)
			return
		}
		w.Header().Set("Content-Type", contentType)
		w.Header().Set("Content-Length", strconv.Itoa(len(data)))
		if r.Method != http.MethodHead {
			_, _ = w.Write(data)
		}
	})
}

func advertisedPort(advertised, bound string) string {
	if advertised != "" {
		return advertised
	}
	return bound
}
