package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

func skipIfDockerUnavailable(t *testing.T) {
	t.Helper()
	conn, err := net.DialTimeout("unix", "/var/run/docker.sock", time.Second)
	if err != nil {
		conn, err = net.DialTimeout("unix", "/Users/crhntr/.docker/run/docker.sock", time.Second)
	}
	if err != nil {
		t.Skip("Docker daemon not available")
	}
	_ = conn.Close()
}

func startPlayground(ctx context.Context, t *testing.T) (string, testcontainers.Container) {
	t.Helper()
	skipIfDockerUnavailable(t)
	ctr, err := testcontainers.Run(ctx, "",
		testcontainers.WithDockerfile(testcontainers.FromDockerfile{
			Context:    "../..",
			Dockerfile: "Dockerfile",
		}),
		testcontainers.WithExposedPorts("8080/tcp"),
		testcontainers.WithWaitStrategy(
			wait.ForHTTP("/").
				WithPort("8080/tcp").
				WithStatusCodeMatcher(func(status int) bool {
					return status == http.StatusOK
				}).
				WithStartupTimeout(120*time.Second),
		),
	)
	if err != nil {
		t.Fatal(err)
	}

	host, err := ctr.Host(ctx)
	if err != nil {
		t.Fatal(err)
	}
	port, err := ctr.MappedPort(ctx, "8080/tcp")
	if err != nil {
		t.Fatal(err)
	}
	return fmt.Sprintf("http://%s:%s", host, port.Port()), ctr
}

func TestGoCompilerRun(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Minute)
	defer cancel()

	containerURL, ctr := startPlayground(ctx, t)
	defer func() { _ = testcontainers.TerminateContainer(ctr) }()

	chromeCtx, cancel := chromedp.NewContext(ctx)
	defer cancel()

	var output string
	err := chromedp.Run(chromeCtx,
		chromedp.Navigate(containerURL),
		chromedp.WaitVisible(`#actions`, chromedp.ByID),
		chromedp.Click(`button[hx-post="/go/run"]`, chromedp.ByQuery),
		chromedp.WaitVisible(`.output`, chromedp.ByQuery),
		chromedp.Poll(`document.querySelector('.output')?.textContent?.includes('Hola')`, nil,
			chromedp.WithPollingTimeout(60*time.Second)),
		chromedp.Text(`.output`, &output, chromedp.ByQuery),
	)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(output, "¡Hola Mundo!") {
		t.Errorf("expected output to contain '¡Hola Mundo!', got: %q", output)
	}
}

// TestIDEFileTabs exercises the IDE-style editor: tree row activation,
// tab open/close preserving file content, new file creation, tree-driven
// delete, format round-trip preserving active state, and the txtar toggle.
func TestIDEFileTabs(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Minute)
	defer cancel()

	containerURL, ctr := startPlayground(ctx, t)
	defer func() { _ = testcontainers.TerminateContainer(ctr) }()

	chromeCtx, cancel := chromedp.NewContext(ctx)
	defer cancel()

	pageURL := containerURL + "/?example=html-template"

	// always auto-accept hx-confirm dialogs
	const overrideConfirm = `window.confirm = () => true; true`

	var (
		treeFiles  []string
		openTabs   string
		activeFile string
		mounted    string
		mainGoLine string
	)

	if err := chromedp.Run(chromeCtx,
		chromedp.Navigate(pageURL),
		chromedp.WaitVisible(`.ide`, chromedp.ByQuery),
		chromedp.Poll(`!!document.querySelector('.CodeMirror')`, nil, chromedp.WithPollingTimeout(20*time.Second)),
		chromedp.Evaluate(overrideConfirm, nil),
		chromedp.Evaluate(`Array.from(document.querySelectorAll('.tree-item')).map(li => li.dataset.file)`, &treeFiles),
		chromedp.Evaluate(`document.querySelector('input[name="active-file"]').value`, &activeFile),
		chromedp.Evaluate(`document.querySelector('input[name="open-tabs"]').value`, &openTabs),
	); err != nil {
		t.Fatal(err)
	}

	wantTree := []string{"body.gohtml", "go.mod", "main.go", "types.go"}
	if !slices.Equal(treeFiles, wantTree) {
		t.Fatalf("tree files: got %v, want %v", treeFiles, wantTree)
	}
	if activeFile != "body.gohtml" {
		t.Fatalf("expected initial active file body.gohtml, got %q", activeFile)
	}
	if openTabs != "body.gohtml" {
		t.Fatalf("expected initial open-tabs to be body.gohtml, got %q", openTabs)
	}

	// Activate main.go from the tree, edit it, switch away, switch back.
	// All clicks below trigger HTMX requests that re-render #editor; the
	// poll waits for the new CodeMirror to mount on the expected file.
	if err := chromedp.Run(chromeCtx,
		chromedp.Click(`.tree-row[data-file="main.go"]`, chromedp.ByQuery),
		chromedp.Poll(`document.querySelector('input[name="active-file"]').value === 'main.go'
			&& !!document.querySelector('.CodeMirror')`, nil, chromedp.WithPollingTimeout(10*time.Second)),
		chromedp.Evaluate(`(() => { const cm = document.querySelector('.CodeMirror').CodeMirror; cm.setValue('// EDITED main.go\n'); cm.save(); return true; })()`, nil),
		chromedp.Click(`.tree-row[data-file="go.mod"]`, chromedp.ByQuery),
		chromedp.Poll(`document.querySelector('input[name="active-file"]').value === 'go.mod'
			&& !!document.querySelector('.CodeMirror')`, nil, chromedp.WithPollingTimeout(10*time.Second)),
		chromedp.Click(`.tab[data-file="main.go"] .tab-name`, chromedp.ByQuery),
		chromedp.Poll(`document.querySelector('input[name="active-file"]').value === 'main.go'
			&& !!document.querySelector('.CodeMirror')`, nil, chromedp.WithPollingTimeout(10*time.Second)),
		chromedp.Evaluate(`document.querySelector('#editor-mount textarea[data-file="main.go"]').value.split('\n')[0]`, &mainGoLine),
	); err != nil {
		t.Fatal(err)
	}
	if mainGoLine != "// EDITED main.go" {
		t.Fatalf("edited content lost across tab switch: first line = %q", mainGoLine)
	}

	// Close the active tab; another open tab should become active and the file should remain in the tree.
	if err := chromedp.Run(chromeCtx,
		chromedp.Click(`.tab[data-file="main.go"] .tab-close`, chromedp.ByQuery),
		chromedp.Poll(`document.querySelector('input[name="active-file"]').value !== 'main.go'
			&& !!document.querySelector('.CodeMirror')`, nil, chromedp.WithPollingTimeout(10*time.Second)),
		chromedp.Evaluate(`document.querySelector('input[name="active-file"]').value`, &activeFile),
		chromedp.Evaluate(`document.querySelector('input[name="open-tabs"]').value`, &openTabs),
		chromedp.Evaluate(`Array.from(document.querySelectorAll('.tree-item')).map(li => li.dataset.file)`, &treeFiles),
	); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(treeFiles, "main.go") {
		t.Fatalf("main.go disappeared from tree after closing tab: %v", treeFiles)
	}
	if activeFile == "main.go" {
		t.Fatal("active file did not change when closing the active tab")
	}
	if strings.Contains(","+openTabs+",", ",main.go,") {
		t.Fatalf("main.go should be removed from open-tabs, got %q", openTabs)
	}

	// Create a new file via the New File input + button. The new file becomes active and gains a tab.
	if err := chromedp.Run(chromeCtx,
		chromedp.Focus(`input[name="new-filename"]`, chromedp.ByQuery),
		chromedp.SendKeys(`input[name="new-filename"]`, "helper.go", chromedp.ByQuery),
		chromedp.Click(`.new-file button`, chromedp.ByQuery),
		chromedp.Poll(`document.querySelector('input[name="active-file"]').value === 'helper.go'`, nil,
			chromedp.WithPollingTimeout(10*time.Second)),
		chromedp.Evaluate(`document.querySelector('input[name="active-file"]').value`, &activeFile),
		chromedp.Evaluate(`document.querySelector('input[name="open-tabs"]').value`, &openTabs),
		chromedp.Evaluate(`document.getElementById('editor-mount')?.querySelector('textarea[data-file]')?.dataset.file`, &mounted),
	); err != nil {
		t.Fatal(err)
	}
	if activeFile != "helper.go" {
		t.Fatalf("active file should be helper.go after create, got %q", activeFile)
	}
	if !strings.Contains(","+openTabs+",", ",helper.go,") {
		t.Fatalf("helper.go should be in open-tabs, got %q", openTabs)
	}
	if mounted != "helper.go" {
		t.Fatalf("mounted textarea should be helper.go, got %q", mounted)
	}

	// Format round-trip preserves active file and open tabs.
	openBefore := openTabs
	activeBefore := activeFile
	if err := chromedp.Run(chromeCtx,
		chromedp.Click(`button[hx-post="/fmt"]`, chromedp.ByQuery),
		chromedp.Poll(`document.querySelector('input[name="active-file"]').value === '`+activeBefore+`'
			&& !!document.querySelector('.CodeMirror')`, nil,
			chromedp.WithPollingTimeout(10*time.Second)),
		chromedp.Evaluate(`document.querySelector('input[name="open-tabs"]').value`, &openTabs),
		chromedp.Evaluate(`document.querySelector('input[name="active-file"]').value`, &activeFile),
	); err != nil {
		t.Fatal(err)
	}
	if openTabs != openBefore || activeFile != activeBefore {
		t.Fatalf("format altered IDE state: open %q→%q, active %q→%q", openBefore, openTabs, activeBefore, activeFile)
	}

	// Delete a file via the tree's delete button.
	if err := chromedp.Run(chromeCtx,
		chromedp.Evaluate(overrideConfirm, nil),
		chromedp.Click(`.tree-item[data-file="types.go"] .tree-delete`, chromedp.ByQuery),
		chromedp.Poll(`!document.querySelector('.tree-item[data-file="types.go"]')`, nil,
			chromedp.WithPollingTimeout(10*time.Second)),
		chromedp.Evaluate(`Array.from(document.querySelectorAll('.tree-item')).map(li => li.dataset.file)`, &treeFiles),
	); err != nil {
		t.Fatal(err)
	}
	if slices.Contains(treeFiles, "types.go") {
		t.Fatalf("types.go should be removed from tree, got %v", treeFiles)
	}

	// Toggle to txtar mode and back; IDE state should survive the round-trip.
	var hasTxtar, hasIDE bool
	if err := chromedp.Run(chromeCtx,
		chromedp.Click(`#toggle-view`, chromedp.ByQuery),
		chromedp.Poll(`!!document.querySelector('textarea.txtar')`, nil, chromedp.WithPollingTimeout(10*time.Second)),
		chromedp.Evaluate(`!!document.querySelector('textarea.txtar')`, &hasTxtar),
		chromedp.Click(`#toggle-view`, chromedp.ByQuery),
		chromedp.Poll(`!!document.querySelector('.ide')`, nil, chromedp.WithPollingTimeout(10*time.Second)),
		chromedp.Evaluate(`!!document.querySelector('.ide')`, &hasIDE),
		chromedp.Evaluate(`document.querySelector('input[name="open-tabs"]').value`, &openTabs),
		chromedp.Evaluate(`document.querySelector('input[name="active-file"]').value`, &activeFile),
	); err != nil {
		t.Fatal(err)
	}
	if !hasTxtar {
		t.Fatal("txtar editor did not appear after first toggle")
	}
	if !hasIDE {
		t.Fatal("IDE did not return after second toggle")
	}
	if openTabs != openBefore || activeFile != activeBefore {
		t.Fatalf("toggle round-trip lost IDE state: open %q vs %q, active %q vs %q",
			openTabs, openBefore, activeFile, activeBefore)
	}
}
