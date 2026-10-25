package ui

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"path"
	"strconv"
	"strings"
	"sync"
)

const staticCacheControl = "public, max-age=0, must-revalidate"

type assetRepresentation struct {
	data []byte
	etag string
}

type staticAsset struct {
	identity assetRepresentation
	gzip     assetRepresentation
}

// Embedded data and compressed variants are immutable and shared by listeners.
// The routing allowlist in NewHandler still controls which files are public.
var staticAssets = sync.OnceValue(func() map[string]staticAsset {
	entries, err := assets.ReadDir("assets")
	if err != nil {
		panic(err)
	}
	result := make(map[string]staticAsset)
	for _, entry := range entries {
		extension := path.Ext(entry.Name())
		if extension != ".css" && extension != ".js" && extension != ".svg" && extension != ".woff2" {
			continue
		}
		filename := "assets/" + entry.Name()
		data, err := assets.ReadFile(filename)
		if err != nil {
			panic(err)
		}
		asset := staticAsset{identity: representation(data)}
		if extension != ".woff2" {
			var compressed bytes.Buffer
			writer, err := gzip.NewWriterLevel(&compressed, gzip.BestCompression)
			if err != nil {
				panic(err)
			}
			if _, err := writer.Write(data); err != nil {
				panic(err)
			}
			if err := writer.Close(); err != nil {
				panic(err)
			}
			asset.gzip = representation(compressed.Bytes())
		}
		result[filename] = asset
	}
	return result
})

func representation(data []byte) assetRepresentation {
	digest := sha256.Sum256(data)
	return assetRepresentation{data: data, etag: `"` + hex.EncodeToString(digest[:]) + `"`}
}

func (asset staticAsset) serve(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", staticCacheControl)
	if asset.gzip.data != nil {
		w.Header().Add("Vary", "Accept-Encoding")
	}
	compressed, acceptable := negotiateEncoding(r.Header.Values("Accept-Encoding"), asset.gzip.data != nil)
	if !acceptable {
		w.Header().Set("Content-Length", "0")
		w.WriteHeader(http.StatusNotAcceptable)
		return
	}
	selected := asset.identity
	if compressed {
		selected = asset.gzip
		w.Header().Set("Content-Encoding", "gzip")
	}
	w.Header().Set("ETag", selected.etag)
	for _, value := range r.Header.Values("If-None-Match") {
		for candidate := range strings.SplitSeq(value, ",") {
			candidate = strings.TrimSpace(candidate)
			if candidate == "*" || strings.TrimPrefix(candidate, "W/") == selected.etag {
				w.WriteHeader(http.StatusNotModified)
				return
			}
		}
	}
	// Serve the complete, bounded representation; do not honor Range requests.
	w.Header().Set("Content-Length", strconv.Itoa(len(selected.data)))
	if r.Method != http.MethodHead {
		_, _ = w.Write(selected.data)
	}
}

// Prefer gzip when qualities tie. Identity remains acceptable unless explicitly
// excluded, including by a wildcard with q=0 (RFC 9110 section 12.5.3).
func negotiateEncoding(values []string, canGzip bool) (compressed, acceptable bool) {
	identity, gzipQuality, wildcard := 1.0, -1.0, -1.0
	identityExplicit := false
	for _, value := range values {
		for token := range strings.SplitSeq(value, ",") {
			encoding, parameters, _ := strings.Cut(token, ";")
			quality := 1.0
			if parameters != "" {
				name, raw, ok := strings.Cut(strings.TrimSpace(parameters), "=")
				parsed, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
				if !ok || !strings.EqualFold(name, "q") || err != nil || !(parsed >= 0 && parsed <= 1) {
					quality = 0
				} else {
					quality = parsed
				}
			}
			switch strings.ToLower(strings.TrimSpace(encoding)) {
			case "gzip":
				gzipQuality = quality
			case "identity":
				identity, identityExplicit = quality, true
			case "*":
				wildcard = quality
			}
		}
	}
	if gzipQuality < 0 {
		gzipQuality = wildcard
	}
	if !identityExplicit && wildcard == 0 {
		identity = 0
	}
	if canGzip && gzipQuality > 0 && gzipQuality >= identity {
		return true, true
	}
	return false, identity > 0
}
