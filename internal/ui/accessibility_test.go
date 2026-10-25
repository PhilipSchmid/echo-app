package ui

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/PhilipSchmid/echo-observatory/v2/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/html"
)

func accessibilityAttribute(node *html.Node, name string) (string, bool) {
	for _, attribute := range node.Attr {
		if attribute.Key == name {
			return attribute.Val, true
		}
	}
	return "", false
}

func accessibilityNodes(document *html.Node) []*html.Node {
	var nodes []*html.Node
	var visit func(*html.Node)
	visit = func(node *html.Node) {
		if node.Type == html.ElementNode {
			nodes = append(nodes, node)
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			visit(child)
		}
	}
	visit(document)
	return nodes
}

func accessibilityText(node *html.Node, visibleOnly bool) string {
	if hidden, _ := accessibilityAttribute(node, "aria-hidden"); hidden == "true" {
		return ""
	}
	if visibleOnly {
		class, _ := accessibilityAttribute(node, "class")
		if _, hidden := accessibilityAttribute(node, "hidden"); hidden || strings.Contains(" "+class+" ", " sr-only ") {
			return ""
		}
	}
	if node.Type == html.TextNode {
		return node.Data
	}
	var text strings.Builder
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		text.WriteString(accessibilityText(child, visibleOnly))
	}
	return text.String()
}

func accessibilityTemplateChild(node *html.Node) bool {
	for parent := node.Parent; parent != nil; parent = parent.Parent {
		if parent.Type == html.ElementNode && parent.Data == "template" {
			return true
		}
	}
	return false
}

func TestRenderedPagesKeepAccessibleDocumentStructure(t *testing.T) {
	handler := NewHandler(&config.Config{PrintHeaders: true, TLS: true, HTTPPort: "8080", MaxRequestSize: 65536}, "HTTP")
	for _, path := range []string{"/ui/", "/ui/workbench/", "/ui/examples/", "/ui/connect/", "/ui/privacy/"} {
		t.Run(path, func(t *testing.T) {
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
			require.Equal(t, http.StatusOK, response.Code)
			document, err := html.Parse(response.Body)
			require.NoError(t, err)
			nodes := accessibilityNodes(document)
			byID := make(map[string]*html.Node)
			labels := make(map[string]bool)
			counts := make(map[string]int)
			for _, node := range nodes {
				counts[node.Data]++
				if id, present := accessibilityAttribute(node, "id"); present {
					require.NotEmpty(t, id)
					require.NotContains(t, byID, id, "IDs must be unique")
					byID[id] = node
				}
				if node.Data == "label" {
					forID, _ := accessibilityAttribute(node, "for")
					labels[forID] = strings.TrimSpace(accessibilityText(node, false)) != ""
				}
			}
			for _, landmark := range []string{"header", "main", "footer", "h1", "title"} {
				assert.Equal(t, 1, counts[landmark], landmark)
			}
			require.Contains(t, byID, "main-content")
			focusable, _ := accessibilityAttribute(byID["main-content"], "tabindex")
			assert.Equal(t, "-1", focusable, "the skip destination must receive keyboard focus")
			lastHeading, currentPageLinks, skipLinks := 0, 0, 0
			navigationNames := make(map[string]bool)
			for _, node := range nodes {
				if accessibilityTemplateChild(node) {
					continue
				}
				for _, reference := range []string{"aria-labelledby", "aria-describedby", "aria-controls"} {
					value, _ := accessibilityAttribute(node, reference)
					for id := range strings.FieldsSeq(value) {
						assert.Contains(t, byID, id, "%s on %s", reference, node.Data)
					}
				}
				switch node.Data {
				case "div", "span":
					if name, present := accessibilityAttribute(node, "aria-label"); present {
						role, _ := accessibilityAttribute(node, "role")
						assert.NotEmpty(t, name)
						assert.NotContains(t, []string{"", "generic", "none", "presentation"}, role, "generic containers cannot expose an accessible name")
					}
				case "html":
					language, _ := accessibilityAttribute(node, "lang")
					assert.Equal(t, "en", language)
				case "title":
					assert.NotEmpty(t, strings.TrimSpace(accessibilityText(node, false)))
				case "meta":
					if name, _ := accessibilityAttribute(node, "name"); name == "viewport" {
						content, _ := accessibilityAttribute(node, "content")
						assert.Contains(t, content, "width=device-width")
						assert.NotContains(t, content, "user-scalable=no")
						assert.NotContains(t, content, "maximum-scale")
					}
				case "nav":
					name, _ := accessibilityAttribute(node, "aria-label")
					assert.NotEmpty(t, name)
					assert.False(t, navigationNames[name], "navigation landmarks need distinct names")
					navigationNames[name] = true
				case "img":
					_, present := accessibilityAttribute(node, "alt")
					assert.True(t, present, "images require an alt attribute, including decorative images")
				case "svg":
					decorative := false
					for ancestor := node; ancestor != nil; ancestor = ancestor.Parent {
						if hidden, _ := accessibilityAttribute(ancestor, "aria-hidden"); hidden == "true" {
							decorative = true
							break
						}
					}
					assert.True(t, decorative, "decorative inline icons must not create unnamed screen-reader objects")
				case "input", "select", "textarea":
					id, _ := accessibilityAttribute(node, "id")
					name, _ := accessibilityAttribute(node, "aria-label")
					reference, _ := accessibilityAttribute(node, "aria-labelledby")
					assert.True(t, labels[id] || name != "" || reference != "", "form control %s requires a programmatic label", id)
				case "a", "button":
					visible := strings.ToLower(strings.Join(strings.Fields(accessibilityText(node, true)), " "))
					name, _ := accessibilityAttribute(node, "aria-label")
					if name != "" && visible != "" {
						assert.Contains(t, strings.ToLower(strings.Join(strings.Fields(name), " ")), visible, "accessible name must include the visible label")
					}
					if current, _ := accessibilityAttribute(node, "aria-current"); current == "page" {
						currentPageLinks++
					}
					if target, _ := accessibilityAttribute(node, "href"); target == "#main-content" {
						skipLinks++
					}
				}
				if len(node.Data) == 2 && node.Data[0] == 'h' && node.Data[1] >= '1' && node.Data[1] <= '6' {
					level := int(node.Data[1] - '0')
					assert.LessOrEqual(t, level, lastHeading+1, "heading levels must not skip")
					lastHeading = level
				}
			}
			assert.Equal(t, 1, currentPageLinks)
			assert.Equal(t, 1, skipLinks)
			assert.Len(t, navigationNames, 2)
		})
	}
}

func TestScrollableCodeBlocksAreNamedKeyboardRegions(t *testing.T) {
	handler := NewHandler(&config.Config{HTTPPort: "8080"}, "HTTP")
	for _, path := range []string{"/ui/", "/ui/workbench/", "/ui/connect/"} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		require.Equal(t, http.StatusOK, response.Code)
		document, err := html.Parse(response.Body)
		require.NoError(t, err)
		for _, node := range accessibilityNodes(document) {
			if node.Data != "pre" {
				continue
			}
			id, _ := accessibilityAttribute(node, "id")
			t.Run(path+"/"+id, func(t *testing.T) {
				role, _ := accessibilityAttribute(node, "role")
				assert.Equal(t, "region", role)
				index, _ := accessibilityAttribute(node, "tabindex")
				assert.Equal(t, "0", index)
				if !accessibilityTemplateChild(node) {
					name, _ := accessibilityAttribute(node, "aria-label")
					assert.NotEmpty(t, name)
				}
			})
		}
	}
}

func TestServiceHelpAndRefreshHaveNativeKeyboardFallbacks(t *testing.T) {
	handler := NewHandler(&config.Config{HTTPPort: "8080"}, "HTTP")
	for _, path := range []string{"/ui/", "/ui/workbench/"} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		require.Equal(t, http.StatusOK, response.Code)
		document, err := html.Parse(response.Body)
		require.NoError(t, err)
		foundStatus, foundLiveStatus := false, false
		for _, node := range accessibilityNodes(document) {
			id, _ := accessibilityAttribute(node, "id")
			if id == "service-status" {
				foundStatus = true
				assert.Equal(t, "summary", node.Data, "status explanation must work with native keyboard and touch activation")
				require.NotNil(t, node.Parent)
				assert.Equal(t, "details", node.Parent.Data)
				name, _ := accessibilityAttribute(node, "aria-label")
				assert.NotEmpty(t, name)
			}
			if _, status := accessibilityAttribute(node, "data-service-status-label"); status {
				foundLiveStatus = true
				role, _ := accessibilityAttribute(node, "role")
				assert.Equal(t, "status", role)
				for ancestor := node.Parent; ancestor != nil; ancestor = ancestor.Parent {
					assert.NotEqual(t, "details", ancestor.Data, "live status must stay available when help is collapsed")
					assert.NotEqual(t, "summary", ancestor.Data, "button descendants can flatten live-region semantics")
				}
			}
			if id == "refresh-visit" {
				assert.Equal(t, "a", node.Data)
				href, _ := accessibilityAttribute(node, "href")
				assert.Equal(t, "./", href, "refresh must work without JavaScript")
			}
		}
		assert.True(t, foundStatus)
		assert.True(t, foundLiveStatus, "service state changes need a persistent live region")
	}
}
