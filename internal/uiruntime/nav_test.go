package uiruntime

import (
	"encoding/json"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
)

// The page layer (decisions 0668 and 0670): links, forms and frames
// between pages that load the runtime, run in testdata/nav.mjs on a fake
// DOM, history and fetch.

type fetch struct {
	Key       string `json:"key"`
	Method    string `json:"method"`
	Multipart bool   `json:"multipart"`
	Body      string `json:"body"`
}

type navReport struct {
	Fetches     []fetch  `json:"fetches"`
	FullLoads   []string `json:"fullLoads"`
	RealSubmits []string `json:"realSubmits"`
	Taken       any      `json:"taken"`
	Path        string   `json:"path"`
	Title       string   `json:"title"`
	Text        string   `json:"text"`
	Kept        bool     `json:"kept"`
	History     int      `json:"history"`
	ScrollB     int      `json:"scrollB"`
	Scroll      int      `json:"scroll"`
	Restoration string   `json:"restoration"`
	BeforeTimer int      `json:"beforeTimer"`
	Typed       string   `json:"typed"`
	TypedText   string   `json:"typedText"`
	HistoryGrew int      `json:"historyGrew"`
	Disabled    bool     `json:"disabledWhile"`
	BusyWhile   bool     `json:"busyWhile"`
	BusyAfter   bool     `json:"busyAfter"`
	BackPath    string   `json:"backPath"`
	Path422     string   `json:"path422"`
	Text422     string   `json:"text422"`
	MultiText   string   `json:"multiText"`
	Side        string   `json:"side"`
	Fetched     int      `json:"fetchedOptOuts"`
}

func nav(t *testing.T, scenario string) navReport {
	t.Helper()
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not found: the runtime runs under Node")
	}
	out, err := exec.Command(node, filepath.Join("testdata", "nav.mjs"), runtimeJS(t), scenario).CombinedOutput()
	if err != nil {
		t.Fatalf("node: %v\n%s", err, out)
	}
	var r navReport
	if err := json.Unmarshal(out, &r); err != nil {
		t.Fatalf("report: %v\n%s", err, out)
	}
	return r
}

func keys(fs []fetch) []string {
	var ks []string
	for _, f := range fs {
		ks = append(ks, f.Method+" "+f.Key)
	}
	return ks
}

// A link to a page with the same runtime is fetched and patched in: no
// full load, a new history entry and title, and a node both pages have
// is the same node.
func TestLinkNavigatesInPlace(t *testing.T) {
	r := nav(t, "link")
	if r.Taken != true || len(r.FullLoads) != 0 || !reflect.DeepEqual(keys(r.Fetches), []string{"GET /b"}) {
		t.Fatalf("taken %v, full loads %q, fetches %q", r.Taken, r.FullLoads, keys(r.Fetches))
	}
	if r.Path != "/b" || r.Title != "B" || r.Text != "page b" || r.History != 2 {
		t.Errorf("path %q, title %q, text %q, history %d: want /b, B, page b, 2", r.Path, r.Title, r.Text, r.History)
	}
	if !r.Kept {
		t.Error("the header both pages have was replaced, not patched")
	}
}

// Back restores the scroll the entry had; the new page started at the top.
func TestBackRestoresScroll(t *testing.T) {
	r := nav(t, "back")
	if r.ScrollB != 0 || r.Scroll != 500 || r.Path != "/" || r.Text != "page a" {
		t.Errorf("scroll on b %d, after back %d at %q (%q): want 0, 500 at / (page a)", r.ScrollB, r.Scroll, r.Path, r.Text)
	}
	if r.Restoration != "manual" || len(r.FullLoads) != 0 {
		t.Errorf("scrollRestoration %q, full loads %q", r.Restoration, r.FullLoads)
	}
}

// A GET form submits in place with its query; with
// data-xo-submit-on-input it submits once its timer runs, replacing the
// history entry.
func TestGetFormInPlace(t *testing.T) {
	r := nav(t, "getform")
	if r.Taken != true || r.Path != "/search?q=tea" || r.Text != "tea" {
		t.Errorf("submit: taken %v, path %q, text %q", r.Taken, r.Path, r.Text)
	}
	if r.BeforeTimer != 1 || r.Typed != "/search?q=teapot" || r.TypedText != "teapot" || r.HistoryGrew != 0 {
		t.Errorf("submit on input: %d fetches before the timer, path %q, text %q, history grew %d: want 1, /search?q=teapot, teapot, 0", r.BeforeTimer, r.Typed, r.TypedText, r.HistoryGrew)
	}
	if len(r.FullLoads) != 0 {
		t.Errorf("full loads %q", r.FullLoads)
	}
}

// A POST answered with a redirect (303) is the page it went to; answered
// 422 it is shown in place with no new URL; while it waits its buttons are
// disabled and the page is busy; a multipart form sends FormData.
func TestPostForm(t *testing.T) {
	r := nav(t, "postform")
	if !r.Disabled || !r.BusyWhile || r.BusyAfter {
		t.Errorf("disabled while waiting %v, busy while %v, busy after %v", r.Disabled, r.BusyWhile, r.BusyAfter)
	}
	if r.Path != "/items/1" || r.Text != "item 1" {
		t.Errorf("303: path %q, text %q: want /items/1, item 1", r.Path, r.Text)
	}
	if r.BackPath != "/new" || r.Path422 != "/new" || r.Text422 != "title taken" {
		t.Errorf("422: path %q, text %q: want /new, title taken", r.Path422, r.Text422)
	}
	if r.MultiText != "uploaded" || len(r.Fetches) != 4 || !r.Fetches[3].Multipart || r.Fetches[0].Body != "title=x" {
		t.Errorf("multipart: text %q, fetches %+v", r.MultiText, r.Fetches)
	}
	if len(r.FullLoads) != 0 || len(r.RealSubmits) != 0 {
		t.Errorf("full loads %q, real submits %q", r.FullLoads, r.RealSubmits)
	}
}

// A link with data-xo-frame patches only that element.
func TestFramePatchesOneElement(t *testing.T) {
	r := nav(t, "frame")
	if r.Path != "/list?p=2" || r.Text != "page 2" || r.Side != "side" || len(r.FullLoads) != 0 {
		t.Errorf("path %q, frame %q, outside it %q, full loads %q: want /list?p=2, page 2, side", r.Path, r.Text, r.Side, r.FullLoads)
	}
}

// data-xo-reload, a target, download, another origin and a modifier key
// are left to the browser; a page with another build of the runtime or
// none, and a failed fetch, load in full; a POST that fails is submitted
// for real, never repeated as a GET.
func TestOptOutsLoadNormally(t *testing.T) {
	r := nav(t, "optouts")
	if !reflect.DeepEqual(r.Taken, []any{false, false, false, false, false, false, false}) || r.Fetched != 0 {
		t.Errorf("taken %v with %d fetches: want none taken, none fetched", r.Taken, r.Fetched)
	}
	if !reflect.DeepEqual(r.FullLoads, []string{"/other", "/plain", "/missing"}) {
		t.Errorf("full loads %q: want /other, /plain, /missing", r.FullLoads)
	}
	if !reflect.DeepEqual(r.RealSubmits, []string{"/missing"}) {
		t.Errorf("real submits %q: want /missing", r.RealSubmits)
	}
}
