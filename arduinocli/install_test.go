package arduinocli

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestArchiveNames(t *testing.T) {
	for _, c := range []struct{ os, arch, want string }{
		{"windows", "amd64", "arduino-cli_latest_Windows_64bit.zip"},
		{"linux", "amd64", "arduino-cli_latest_Linux_64bit.tar.gz"},
		{"darwin", "arm64", "arduino-cli_latest_macOS_ARM64.tar.gz"},
	} {
		if got, err := Archive(c.os, c.arch); err != nil || got != c.want {
			t.Errorf("%s/%s: %q %v", c.os, c.arch, got, err)
		}
	}
	if _, err := Archive("plan9", "amd64"); err == nil {
		t.Error("plan9 has a release?")
	}
}

func zipWith(t *testing.T, name string, data []byte) []byte {
	var b bytes.Buffer
	zw := zip.NewWriter(&b)
	w, _ := zw.Create("LICENSE.txt")
	w.Write([]byte("licence"))
	w, _ = zw.Create(name)
	w.Write(data)
	zw.Close()
	return b.Bytes()
}

func tgzWith(t *testing.T, name string, data []byte) []byte {
	var b bytes.Buffer
	gz := gzip.NewWriter(&b)
	tw := tar.NewWriter(gz)
	tw.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(data)), Typeflag: tar.TypeReg})
	tw.Write(data)
	tw.Close()
	gz.Close()
	return b.Bytes()
}

// The release fetched from a server, unpacked into the folder, then the
// core and the Servo library installed through it, in that order.
func TestSetupDownloadsAndInstalls(t *testing.T) {
	tool := []byte("the tool")
	files := map[string][]byte{
		"/arduino-cli_latest_Windows_64bit.zip":  zipWith(t, "arduino-cli.exe", tool),
		"/arduino-cli_latest_Linux_64bit.tar.gz": tgzWith(t, "arduino-cli", tool),
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, ok := files[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Write(b)
	}))
	defer srv.Close()
	for _, goos := range []string{"windows", "linux"} {
		dir := t.TempDir()
		var ran []string
		var stages []string
		in := Installer{BaseURL: srv.URL, Dir: dir, GOOS: goos, GOARCH: "amd64",
			Run: func(_ context.Context, exe string, args ...string) ([]byte, error) {
				ran = append(ran, filepath.Base(exe)+" "+strings.Join(args, " "))
				return nil, nil
			}}
		exe, err := in.Setup(context.Background(), func(stage string, _ float64) {
			if len(stages) == 0 || stages[len(stages)-1] != stage {
				stages = append(stages, stage)
			}
		})
		if err != nil {
			t.Fatalf("%s: %v", goos, err)
		}
		b, err := os.ReadFile(exe)
		if err != nil || !bytes.Equal(b, tool) {
			t.Fatalf("%s: unpacked %q (%v)", goos, b, err)
		}
		if len(ran) != 3 || !strings.HasSuffix(ran[1], "core install arduino:avr") || !strings.HasSuffix(ran[2], "lib install Servo") {
			t.Fatalf("%s: ran %q", goos, ran)
		}
		if stages[0] != "downloading arduino-cli" || stages[len(stages)-1] != "done" {
			t.Fatalf("%s: stages %q", goos, stages)
		}
	}
	// a missing release says so
	in := Installer{BaseURL: srv.URL, Dir: t.TempDir(), GOOS: "darwin", GOARCH: "amd64"}
	if _, err := in.Download(context.Background(), func(string, float64) {}); err == nil || !strings.Contains(err.Error(), "404") {
		t.Fatalf("missing release: %v", err)
	}
}

// The real thing, from Arduino (FORMA_ARDUINO_DOWNLOAD=1; it fetches ~30
// MB): the release for this system downloaded into an empty folder, as on
// a fresh machine, and run; and the archives for the other systems Forma
// is built for are there.
func TestRealDownload(t *testing.T) {
	if os.Getenv("FORMA_ARDUINO_DOWNLOAD") == "" {
		t.Skip("set FORMA_ARDUINO_DOWNLOAD=1 to download arduino-cli from Arduino")
	}
	ctx := context.Background()
	exe, err := Installer{Dir: t.TempDir()}.Download(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	out, err := runTool(ctx, exe, "version")
	if err != nil || !strings.Contains(string(out), "arduino-cli") {
		t.Fatalf("%s version: %v %s", exe, err, out)
	}
	t.Logf("%s", strings.TrimSpace(string(out)))
	for _, p := range [][2]string{{"windows", "amd64"}, {"linux", "amd64"}, {"darwin", "arm64"}, {"darwin", "amd64"}} {
		name, err := Archive(p[0], p[1])
		if err != nil {
			t.Fatal(err)
		}
		req, _ := http.NewRequestWithContext(ctx, "HEAD", BaseURL+"/"+name, nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("%s: %s", name, resp.Status)
		}
		t.Logf("%s: %s, %d bytes", name, resp.Status, resp.ContentLength)
	}
}
