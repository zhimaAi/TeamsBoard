// Command build cross-compiles the Go sidecar and optionally embeds web/dist.
package main

import (
	"archive/zip"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func main() {
	goos := flag.String("goos", "", "target OS: windows, darwin or linux")
	goarch := flag.String("goarch", "", "target architecture: amd64 or arm64")
	name := flag.String("name", "client", "binary base name")
	dist := flag.String("dist", "dist", "artifact output directory")
	cmdPath := flag.String("cmd", "./cmd/client", "Go main package")
	web := flag.String("web", "web/dist", "frontend output embedded when present")
	modRoot := flag.String("modroot", ".", "Go module root")
	output := flag.String("output", "", "exact output path for Electron staging")
	config := flag.String("config", "", "embedded default config name (config_<name>.ini); empty means config.ini")
	flag.Parse()

	if *goos == "" || *goarch == "" {
		fatal("-goos and -goarch are required")
	}

	version := gitValue(*modRoot, []string{"describe", "--tags", "--always", "--dirty"}, "dev")
	commit := gitValue(*modRoot, []string{"rev-parse", "--short", "HEAD"}, "unknown")
	buildTime := time.Now().UTC().Format(time.RFC3339)

	outName := fmt.Sprintf("%s-%s-%s", *name, *goos, *goarch)
	if *goos == "windows" {
		outName += ".exe"
	}
	binPath := filepath.Join(*dist, outName)
	if strings.TrimSpace(*output) != "" {
		binPath = *output
	}
	must(os.MkdirAll(filepath.Dir(binPath), 0o755))

	ldflags := []string{
		"-s", "-w",
		fmt.Sprintf("-X main.Version=%s", version),
		fmt.Sprintf("-X main.Commit=%s", commit),
		fmt.Sprintf("-X main.BuildTime=%s", buildTime),
	}
	if strings.TrimSpace(*config) != "" {
		ldflags = append(ldflags, fmt.Sprintf("-X goteams-client/internal/bootstrap.DefaultConfigName=%s", *config))
	}
	args := []string{"build", "-trimpath", "-ldflags", strings.Join(ldflags, " "), "-o", binPath}
	if webDirExists(*modRoot, *web) {
		args = append(args, "-tags", "assets_web")
		fmt.Println("==> web/dist detected; embedding renderer assets")
		warnIfWebStale(*modRoot, *web)
	}
	args = append(args, *cmdPath)

	build := exec.Command("go", args...)
	build.Dir = *modRoot
	build.Env = append(os.Environ(), "GOOS="+*goos, "GOARCH="+*goarch, "CGO_ENABLED=0")
	build.Stdout = os.Stdout
	build.Stderr = os.Stderr
	fmt.Printf("==> Building %s/%s -> %s\n", *goos, *goarch, binPath)
	if err := build.Run(); err != nil {
		fatal("go build failed: %v", err)
	}
	if *goos != "windows" {
		must(os.Chmod(binPath, 0o755))
	}

	if *goos == "darwin" && strings.TrimSpace(*output) == "" {
		zipPath := binPath + ".zip"
		if err := zipBinary(binPath, zipPath); err != nil {
			fatal("zip failed: %v", err)
		}
	}
	fmt.Printf("==> Done: %s\n", binPath)
}

func zipBinary(binPath, zipPath string) error {
	data, err := os.ReadFile(binPath)
	if err != nil {
		return err
	}
	f, err := os.Create(zipPath)
	if err != nil {
		return err
	}
	defer f.Close()
	zw := zip.NewWriter(f)
	header := &zip.FileHeader{Name: filepath.Base(binPath), Method: zip.Deflate}
	header.SetMode(0o755)
	w, err := zw.CreateHeader(header)
	if err != nil {
		return err
	}
	if _, err := w.Write(data); err != nil {
		return err
	}
	return zw.Close()
}

func webDirExists(root, web string) bool {
	info, err := os.Stat(filepath.Join(root, web))
	return err == nil && info.IsDir()
}

func warnIfWebStale(root, web string) {
	distInfo, err := os.Stat(filepath.Join(root, web, "index.html"))
	if err != nil {
		fmt.Println("!!! WARNING: web/dist/index.html is missing")
		return
	}
	var newest time.Time
	_ = filepath.WalkDir(filepath.Join(root, "web", "src"), func(_ string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil || entry.IsDir() {
			return nil
		}
		if info, statErr := entry.Info(); statErr == nil && info.ModTime().After(newest) {
			newest = info.ModTime()
		}
		return nil
	})
	if newest.After(distInfo.ModTime()) {
		fmt.Println("!!! WARNING: web/dist is older than web/src; run task web:build first")
	}
}

func gitValue(dir string, args []string, fallback string) string {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return fallback
	}
	return strings.TrimSpace(string(out))
}

func fatal(format string, values ...any) {
	fmt.Fprintf(os.Stderr, "Error: "+format+"\n", values...)
	os.Exit(1)
}

func must(err error) {
	if err != nil {
		fatal("%v", err)
	}
}
