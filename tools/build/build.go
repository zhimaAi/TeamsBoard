// Command build is a cross-platform build helper invoked by Taskfile.
//
// It is responsible for:
//  1. Obtain version/commit info via git and inject via -ldflags -X;
//  2. Cross-compile for the target platform with CGO_ENABLED=0 (pure Go, no C toolchain);
//  3. If web/dist exists, add -tags assets_web to embed the frontend into the binary as well;
//  4. Produce a single executable to dist/<name>-<goos>-<goarch>[.exe].
//
// All resources (migrations/skills/configs, and optional web/dist) are embedded into the binary at compile time via
// the root assets package using go:embed, so the artifact is a single file needing no sibling directories.
// Go is used instead of a shell script to avoid OS-specific command differences.
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
	var (
		goos    = flag.String("goos", "", "目标操作系统 (windows/darwin/linux)")
		goarch  = flag.String("goarch", "", "目标架构 (amd64/arm64)")
		name    = flag.String("name", "client", "二进制基础名")
		dist    = flag.String("dist", "dist", "产物输出目录")
		cmd     = flag.String("cmd", "./cmd/client", "main 包路径")
		web     = flag.String("web", "web/dist", "前端构建产物目录；存在则通过 -tags assets_web 嵌入二进制")
		modRoot = flag.String("modroot", ".", "模块根目录 (用于执行 git/go)")
	)
	flag.Parse()

	if *goos == "" || *goarch == "" {
		fatal("必须指定 -goos 与 -goarch")
	}

	// Compute version info
	version := gitValue(*modRoot, []string{"describe", "--tags", "--always", "--dirty"}, "dev")
	commit := gitValue(*modRoot, []string{"rev-parse", "--short", "HEAD"}, "unknown")
	buildTime := time.Now().UTC().Format(time.RFC3339)

	outName := fmt.Sprintf("%s-%s-%s", *name, *goos, *goarch)
	if *goos == "windows" {
		outName += ".exe"
	}
	binPath := filepath.Join(*dist, outName)

	// Prepare the artifact directory
	must(os.MkdirAll(*dist, 0o755))

	// Cross-compile
	ldflags := strings.Join([]string{
		"-s", "-w",
		fmt.Sprintf("-X main.Version=%s", version),
		fmt.Sprintf("-X main.Commit=%s", commit),
		fmt.Sprintf("-X main.BuildTime=%s", buildTime),
	}, " ")
	args := []string{
		"build", "-trimpath",
		"-ldflags", ldflags,
		"-o", binPath,
	}
	// If the frontend is built, embed it in the binary (resources unified into the executable). Note: -tags must precede the package path.
	if webDirExists(*modRoot, *web) {
		args = append(args, "-tags", "assets_web")
		fmt.Println("==> Detected web/dist, will embed frontend with -tags assets_web")
		warnIfWebStale(*modRoot, *web)
	}
	args = append(args, *cmd)

	build := exec.Command("go", args...)
	build.Dir = *modRoot
	build.Env = append(os.Environ(),
		"GOOS="+*goos,
		"GOARCH="+*goarch,
		"CGO_ENABLED=0",
	)
	build.Stdout = os.Stdout
	build.Stderr = os.Stderr
	fmt.Printf("==> Building %s/%s -> %s\n", *goos, *goarch, binPath)
	if err := build.Run(); err != nil {
		fatal("go build 失败: %v", err)
	}

	// macOS artifacts are additionally packaged as zip (entry permission 0755, runnable directly after unzip on distribution)
	if *goos == "darwin" {
		zipPath := binPath + ".zip"
		fmt.Printf("==> Packaging %s\n", zipPath)
		if err := zipBinary(binPath, zipPath); err != nil {
			fatal("打包 zip 失败: %v", err)
		}
	}

	fmt.Printf("==> Done: single-file executable %s\n", binPath)
}

// zipBinary packages the binary into a zip and sets entry permission to 0755.
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
	hdr := &zip.FileHeader{
		Name:   filepath.Base(binPath),
		Method: zip.Deflate,
	}
	hdr.SetMode(0o755)
	w, err := zw.CreateHeader(hdr)
	if err != nil {
		return err
	}
	if _, err := w.Write(data); err != nil {
		return err
	}
	return zw.Close()
}

func webDirExists(modRoot, web string) bool {
	info, err := os.Stat(filepath.Join(modRoot, web))
	return err == nil && info.IsDir()
}

// warnIfWebStale 比较 web/src 最新改动时间与 web/dist/index.html 的时间，
// 若产物比源码旧则给出醒目告警，避免静默把过期前端 embed 进二进制。
func warnIfWebStale(modRoot, web string) {
	dist, err := os.Stat(filepath.Join(modRoot, web, "index.html"))
	if err != nil {
		fmt.Println("!!! WARNING: web/dist/index.html 不存在，嵌入的前端可能不完整")
		return
	}
	var newest time.Time
	srcRoot := filepath.Join(modRoot, "web", "src")
	_ = filepath.WalkDir(srcRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if info, statErr := d.Info(); statErr == nil && info.ModTime().After(newest) {
			newest = info.ModTime()
		}
		return nil
	})
	if newest.After(dist.ModTime()) {
		fmt.Printf("!!! WARNING: web/dist 已过期（源码 %s 晚于产物 %s），请先执行 `task web:build`\n",
			newest.Format(time.RFC3339), dist.ModTime().Format(time.RFC3339))
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

func fatal(format string, a ...any) {
	fmt.Fprintf(os.Stderr, "Error: "+format+"\n", a...)
	os.Exit(1)
}

func must(err error) {
	if err != nil {
		fatal("%v", err)
	}
}
