package ingest

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"go.uber.org/zap"
)

// tarballMirrors tarball 降级下载源（git clone 受 GFW 指纹封锁/代理不稳定时的兜底）。
// Go net/http 的 TLS 指纹目前可直连 codeload；镜像源覆盖国内 CDN，按序尝试。
var tarballMirrors = []string{
	"https://codeload.github.com/{owner}/{repo}/tar.gz/refs/heads/{branch}",
	"https://mirror.ghproxy.com/https://github.com/{owner}/{repo}/archive/refs/heads/{branch}.tar.gz",
	"https://gh-proxy.com/https://github.com/{owner}/{repo}/archive/refs/heads/{branch}.tar.gz",
}

var githubURLRe = regexp.MustCompile(`^https?://github\.com/([^/]+)/([^/]+?)(?:\.git)?/?$`)

// cloneViaTarball tarball 降级克隆：HTTP 下载 tar.gz → 解压到 destDir（剥掉顶层目录）。
// 注意：不产生 .git 元数据——ingest pipeline（ParseFiles/ChunkFiles）不依赖 git 历史；
// 但 refresh 的 FetchAndReset 需要真实 git 仓库，tarball 摄取的仓库 refresh 会在 fetching 阶段失败（dev 取舍）。
func (c *GitCloner) cloneViaTarball(ctx context.Context, url, branch, destDir string) error {
	m := githubURLRe.FindStringSubmatch(url)
	if m == nil {
		return fmt.Errorf("not a github url: %s", url)
	}
	owner, repo := m[1], m[2]
	if branch == "" {
		branch = "HEAD"
	}

	client := &http.Client{Timeout: 5 * time.Minute} // Transport 默认遵循 HTTP(S)_PROXY 环境变量
	var lastErr error
	for _, tpl := range tarballMirrors {
		u := strings.NewReplacer("{owner}", owner, "{repo}", repo, "{branch}", branch).Replace(tpl)
		err := downloadAndExtract(ctx, client, u, destDir)
		if err == nil {
			c.logger.Info("tarball fallback clone succeeded", zap.String("url", u), zap.String("dest", destDir))
			return nil
		}
		lastErr = err
		c.logger.Warn("tarball mirror failed", zap.String("url", u), zap.Error(err))
	}
	return fmt.Errorf("all tarball mirrors failed: %w", lastErr)
}

// downloadAndExtract 下载 tar.gz 并解压到 destDir（剥掉第一层目录 <repo>-<ref>/）。
func downloadAndExtract(ctx context.Context, client *http.Client, url, destDir string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("http %d", resp.StatusCode)
	}

	gz, err := gzip.NewReader(resp.Body)
	if err != nil {
		return fmt.Errorf("gzip reader: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("tar read: %w", err)
		}
		// 剥掉顶层目录；防路径穿越（反 AI 错误 #11）。
		name := stripTopDir(hdr.Name)
		if name == "" || strings.Contains(name, "..") {
			continue
		}
		target := filepath.Join(destDir, name)
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			f, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
			if err != nil {
				return err
			}
			if _, err := io.Copy(f, tr); err != nil {
				_ = f.Close()
				return err
			}
			if err := f.Close(); err != nil {
				return err
			}
		}
	}
	return nil
}

// stripTopDir 去掉 tar 条目路径的第一层目录（GitHub tarball 形如 <repo>-<sha>/path）。
func stripTopDir(name string) string {
	name = strings.TrimPrefix(filepath.ToSlash(name), "./")
	parts := strings.SplitN(name, "/", 2)
	if len(parts) < 2 {
		return ""
	}
	return parts[1]
}
