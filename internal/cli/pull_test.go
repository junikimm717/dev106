package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestPullDisplaySkipsProgressTicksWithoutTTY(t *testing.T) {
	var buf bytes.Buffer
	d := &pullDisplay{
		out:    &buf,
		tty:    false,
		lines:  make(map[string]string),
		status: make(map[string]string),
	}

	d.handle(pullMessage{ID: "dfad4f069815", Status: "Downloading", Progress: "[=>] 1B/10B"})
	d.handle(pullMessage{ID: "dfad4f069815", Status: "Downloading", Progress: "[==>] 5B/10B"})
	d.handle(pullMessage{ID: "dfad4f069815", Status: "Downloading", Progress: "[====>] 10B/10B"})
	d.handle(pullMessage{ID: "dfad4f069815", Status: "Download complete"})
	d.handle(pullMessage{ID: "dfad4f069815", Status: "Extracting", Progress: "[=>] 1B/10B"})
	d.handle(pullMessage{ID: "dfad4f069815", Status: "Extracting", Progress: "[====>] 10B/10B"})
	d.handle(pullMessage{ID: "dfad4f069815", Status: "Pull complete"})
	d.handle(pullMessage{Status: "Status: Downloaded newer image for example:latest"})

	got := buf.String()
	want := strings.Join([]string{
		"dfad4f069815: Downloading",
		"dfad4f069815: Download complete",
		"dfad4f069815: Extracting",
		"dfad4f069815: Pull complete",
		"Status: Downloaded newer image for example:latest",
		"",
	}, "\n")
	if got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestLayerLineIncludesProgress(t *testing.T) {
	got := layerLine(pullMessage{ID: "dfad4f069815abcd", Status: "Downloading", Progress: "  [==>] 5B/10B"})
	if !strings.Contains(got, "dfad4f069815: Downloading") {
		t.Fatalf("line = %q", got)
	}
	if !strings.Contains(got, "[==>] 5B/10B") {
		t.Fatalf("missing progress: %q", got)
	}
}
