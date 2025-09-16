package github_releases

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/OpenListTeam/OpenList/v4/drivers/base"
	"github.com/go-resty/resty/v2"
	log "github.com/sirupsen/logrus"
)

// 发�?GET 请求
func (d *GithubReleases) GetRequest(url string) (*resty.Response, error) {
	req := base.RWithProxy(d.DriverProxyAddr)
	req.SetHeader("Accept", "application/vnd.github+json")
	req.SetHeader("X-GitHub-Api-Version", "2022-11-28")
	if d.Addition.Token != "" {
		req.SetHeader("Authorization", fmt.Sprintf("Bearer %s", d.Addition.Token))
	}
	res, err := req.Get(url)
	if err != nil {
		return nil, err
	}
	if res.StatusCode() != 200 {
		log.Warn("failed to get request: ", res.StatusCode(), res.String())
	}
	return res, nil
}

// 解析挂载结构
func (d *GithubReleases) ParseRepos(text string) ([]MountPoint, error) {
	lines := strings.Split(text, "\n")
	points := make([]MountPoint, 0)
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Split(line, ":")
		path, repo := "", ""
		if len(parts) == 1 {
			path = "/"
			repo = parts[0]
		} else if len(parts) == 2 {
			path = fmt.Sprintf("/%s", strings.Trim(parts[0], "/"))
			repo = parts[1]
		} else {
			return nil, fmt.Errorf("invalid format: %s", line)
		}

		points = append(points, MountPoint{
			Point:    path,
			Repo:     repo,
			Release:  nil,
			Releases: nil,
		})
	}
	d.points = points
	return points, nil
}

// 获取下一级目�?
func GetNextDir(wholePath string, basePath string) string {
	basePath = fmt.Sprintf("%s/", strings.TrimRight(basePath, "/"))
	if !strings.HasPrefix(wholePath, basePath) {
		return ""
	}
	remainingPath := strings.TrimLeft(strings.TrimPrefix(wholePath, basePath), "/")
	if remainingPath != "" {
		parts := strings.Split(remainingPath, "/")
		nextDir := parts[0]
		if strings.HasPrefix(wholePath, strings.TrimRight(basePath, "/")+"/"+nextDir) {
			return nextDir
		}
	}
	return ""
}

// 判断当前目录是否是目标目录的祖先目录
func IsAncestorDir(parentDir string, targetDir string) bool {
	absTargetDir, _ := filepath.Abs(targetDir)
	absParentDir, _ := filepath.Abs(parentDir)
	return strings.HasPrefix(absTargetDir, absParentDir)
}
