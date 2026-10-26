package daemon

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"maps"
	"mime"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"go.yaml.in/yaml/v4"

	"github.com/tarik02-org/agx/internal/securefs"
)

// Icons the web UI draws for definitions arrive as data URIs, so that the UI loads nothing from
// elsewhere: a skill's from its agents/openai.yaml, an MCP server's from what it declares when it
// starts, remembered for when it is not running.

// maxIconBytes bounds an icon's image; icons are drawn at a few pixels.
const maxIconBytes = 64 << 10

// iconFetchTimeout bounds fetching an icon an MCP server names by URL.
const iconFetchTimeout = 5 * time.Second

var (
	brandColor = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)
	// iconTypes are the image types drawn as icons, by file extension.
	iconTypes = map[string]string{
		".png":  "image/png",
		".svg":  "image/svg+xml",
		".jpg":  "image/jpeg",
		".jpeg": "image/jpeg",
		".webp": "image/webp",
		".gif":  "image/gif",
	}
	iconDataURI = regexp.MustCompile(`^data:image/(png|svg\+xml|jpeg|webp|gif);base64,[A-Za-z0-9+/]+={0,2}$`)
)

func dataURI(mimeType string, data []byte) string {
	return "data:" + mimeType + ";base64," + base64.StdEncoding.EncodeToString(data)
}

// skillInterface reads what a skill's agents/openai.yaml, the interface metadata Codex reads,
// declares for drawing it: a #rrggbb brand color and a small icon as a data URI. Either is empty
// when missing or unusable; the icon must be an image within the skill's directory.
func skillInterface(dir string) (color, icon string) {
	document, err := os.ReadFile(filepath.Join(dir, "agents", "openai.yaml"))
	if err != nil {
		return "", ""
	}
	var metadata struct {
		Interface struct {
			BrandColor string `yaml:"brand_color"`
			IconSmall  string `yaml:"icon_small"`
		} `yaml:"interface"`
	}
	if yaml.Unmarshal(document, &metadata) != nil {
		return "", ""
	}
	if brandColor.MatchString(metadata.Interface.BrandColor) {
		color = metadata.Interface.BrandColor
	}
	return color, skillIcon(dir, metadata.Interface.IconSmall)
}

// skillIcon reads an icon file named relative to a skill's directory, through a root confined
// to it, so the name cannot reach outside.
func skillIcon(dir, name string) string {
	mimeType, ok := iconTypes[strings.ToLower(path.Ext(name))]
	if name == "" || !ok {
		return ""
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return ""
	}
	defer func() { _ = root.Close() }()
	file, err := root.Open(path.Clean(strings.TrimPrefix(filepath.ToSlash(name), "./")))
	if err != nil {
		return ""
	}
	defer func() { _ = file.Close() }()
	data, err := io.ReadAll(io.LimitReader(file, maxIconBytes+1))
	if err != nil || len(data) > maxIconBytes {
		return ""
	}
	return dataURI(mimeType, data)
}

// mcpIcons remembers the icon each MCP server declared when it last started, in the cache, so
// the UI can draw it while the server is not running.
type mcpIcons struct {
	mu     sync.Mutex
	path   string
	client *http.Client
	// icons maps server IDs to their icons; nil until read from the file.
	icons map[string]mcpIcon
}

type mcpIcon struct {
	// Source is the icon's src as the server declared it, so an unchanged URL is not fetched
	// again.
	Source string `json:"src"`
	Icon   string `json:"icon"`
}

func newMCPIcons(path string) *mcpIcons { return &mcpIcons{path: path, client: http.DefaultClient} }

// load reads the file once; a missing or broken one is an empty cache. c.mu is held.
func (c *mcpIcons) load() {
	if c.icons != nil {
		return
	}
	c.icons = map[string]mcpIcon{}
	data, err := os.ReadFile(c.path)
	if err != nil {
		return
	}
	var stored map[string]mcpIcon
	if json.Unmarshal(data, &stored) == nil {
		for id, icon := range stored {
			if iconDataURI.MatchString(icon.Icon) {
				c.icons[id] = icon
			}
		}
	}
}

// get returns server id's remembered icon, or empty. A nil cache remembers nothing.
func (c *mcpIcons) get(id string) string {
	if c == nil {
		return ""
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.load()
	return c.icons[id].Icon
}

// update replaces server id's icon with the best of those it declared now, and returns it;
// a server that declares none forgets its icon.
func (c *mcpIcons) update(ctx context.Context, id string, declared []mcp.Icon) string {
	if c == nil {
		return ""
	}
	c.mu.Lock()
	c.load()
	known := c.icons[id]
	c.mu.Unlock()

	chosen, ok := chooseIcon(declared)
	next := mcpIcon{}
	switch {
	case !ok:
	case chosen.Source == known.Source:
		next = known
	default:
		if icon := c.resolve(ctx, chosen); icon != "" {
			next = mcpIcon{Source: chosen.Source, Icon: icon}
		}
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if next == c.icons[id] {
		return next.Icon
	}
	if next.Icon == "" {
		delete(c.icons, id)
	} else {
		c.icons[id] = next
	}
	// An icon that is not remembered is drawn from the server's kind next time; nothing breaks.
	_ = securefs.WriteJSON(c.path, c.icons)
	return next.Icon
}

// chooseIcon picks the icon to draw on the UI's dark background: one made for dark themes,
// then one for any theme, then one for light themes.
func chooseIcon(icons []mcp.Icon) (mcp.Icon, bool) {
	rank := func(icon mcp.Icon) int {
		switch icon.Theme {
		case "dark":
			return 0
		case "light":
			return 2
		default:
			return 1
		}
	}
	usable := slices.DeleteFunc(slices.Clone(icons), func(icon mcp.Icon) bool {
		return !iconDataURI.MatchString(icon.Source) && !strings.HasPrefix(icon.Source, "https://")
	})
	if len(usable) == 0 {
		return mcp.Icon{}, false
	}
	slices.SortStableFunc(usable, func(a, b mcp.Icon) int { return rank(a) - rank(b) })
	return usable[0], true
}

// resolve returns an icon as a data URI: as given, or fetched from its HTTPS URL; empty when it
// cannot be had or is not a small image.
func (c *mcpIcons) resolve(ctx context.Context, icon mcp.Icon) string {
	if iconDataURI.MatchString(icon.Source) {
		if len(icon.Source) > base64.StdEncoding.EncodedLen(maxIconBytes)+64 {
			return ""
		}
		return icon.Source
	}
	ctx, cancel := context.WithTimeout(ctx, iconFetchTimeout)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, icon.Source, nil)
	if err != nil {
		return ""
	}
	response, err := c.client.Do(request)
	if err != nil {
		return ""
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return ""
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, maxIconBytes+1))
	if err != nil || len(data) > maxIconBytes {
		return ""
	}
	known := slices.Collect(maps.Values(iconTypes))
	mimeType, _, _ := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if !slices.Contains(known, mimeType) {
		mimeType = icon.MIMEType
	}
	if !slices.Contains(known, mimeType) {
		return ""
	}
	return dataURI(mimeType, data)
}
