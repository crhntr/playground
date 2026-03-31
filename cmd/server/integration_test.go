package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
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
