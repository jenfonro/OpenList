package github_releases

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/OpenListTeam/OpenList/v4/pkg/utils"
	"github.com/go-resty/resty/v2"
)

type MountPoint struct {
	Point     string      // 挂载�?
	Repo      string      // 仓库�?owner/repo
	Release   *Release    // Release 指针 latest
	Releases  *[]Release  // []Release 指针
	OtherFile *[]FileInfo // 仓库根目录下的其他文�?
}

// 请求最新版�?
func (m *MountPoint) RequestRelease(get func(url string) (*resty.Response, error), refresh bool) {
	if m.Repo == "" {
		return
	}

	if m.Release == nil || refresh {
		resp, _ := get("https://api.github.com/repos/" + m.Repo + "/releases/latest")
		m.Release = new(Release)
		json.Unmarshal(resp.Body(), m.Release)
	}
}

// 请求所有版�?
func (m *MountPoint) RequestReleases(get func(url string) (*resty.Response, error), refresh bool) {
	if m.Repo == "" {
		return
	}

	if m.Releases == nil || refresh {
		resp, _ := get("https://api.github.com/repos/" + m.Repo + "/releases")
		m.Releases = new([]Release)
		json.Unmarshal(resp.Body(), m.Releases)
	}
}

// 获取最新版�?
func (m *MountPoint) GetLatestRelease() []File {
	files := make([]File, 0)
	for _, asset := range m.Release.Assets {
		files = append(files, File{
			Path:     m.Point + "/" + asset.Name,
			FileName: asset.Name,
			Size:     asset.Size,
			Type:     "file",
			UpdateAt: asset.UpdatedAt,
			CreateAt: asset.CreatedAt,
			Url:      asset.BrowserDownloadUrl,
		})
	}
	return files
}

// 获取最新版本大�?
func (m *MountPoint) GetLatestSize() int64 {
	size := int64(0)
	for _, asset := range m.Release.Assets {
		size += asset.Size
	}
	return size
}

// 获取所有版�?
func (m *MountPoint) GetAllVersion() []File {
	files := make([]File, 0)
	for _, release := range *m.Releases {
		file := File{
			Path:     m.Point + "/" + release.TagName,
			FileName: release.TagName,
			Size:     m.GetSizeByTagName(release.TagName),
			Type:     "dir",
			UpdateAt: release.PublishedAt,
			CreateAt: release.CreatedAt,
			Url:      release.HtmlUrl,
		}
		for _, asset := range release.Assets {
			file.Size += asset.Size
		}
		files = append(files, file)
	}
	return files
}

// 根据版本号获取版�?
func (m *MountPoint) GetReleaseByTagName(tagName string) []File {
	for _, item := range *m.Releases {
		if item.TagName == tagName {
			files := make([]File, 0)
			for _, asset := range item.Assets {
				files = append(files, File{
					Path:     m.Point + "/" + tagName + "/" + asset.Name,
					FileName: asset.Name,
					Size:     asset.Size,
					Type:     "file",
					UpdateAt: asset.UpdatedAt,
					CreateAt: asset.CreatedAt,
					Url:      asset.BrowserDownloadUrl,
				})
			}
			return files
		}
	}
	return nil
}

// 根据版本号获取版本大�?
func (m *MountPoint) GetSizeByTagName(tagName string) int64 {
	if m.Releases == nil {
		return 0
	}
	for _, item := range *m.Releases {
		if item.TagName == tagName {
			size := int64(0)
			for _, asset := range item.Assets {
				size += asset.Size
			}
			return size
		}
	}
	return 0
}

// 获取所有版本大�?
func (m *MountPoint) GetAllVersionSize() int64 {
	if m.Releases == nil {
		return 0
	}
	size := int64(0)
	for _, release := range *m.Releases {
		for _, asset := range release.Assets {
			size += asset.Size
		}
	}
	return size
}

func (m *MountPoint) GetOtherFile(get func(url string) (*resty.Response, error), refresh bool) []File {
	if m.OtherFile == nil || refresh {
		resp, _ := get("https://api.github.com/repos/" + m.Repo + "/contents")
		m.OtherFile = new([]FileInfo)
		json.Unmarshal(resp.Body(), m.OtherFile)
	}

	files := make([]File, 0)
	defaultTime := "1970-01-01T00:00:00Z"
	for _, file := range *m.OtherFile {
		if strings.HasSuffix(file.Name, ".md") || strings.HasPrefix(file.Name, "LICENSE") {
			files = append(files, File{
				Path:     m.Point + "/" + file.Name,
				FileName: file.Name,
				Size:     file.Size,
				Type:     "file",
				UpdateAt: defaultTime,
				CreateAt: defaultTime,
				Url:      file.DownloadUrl,
			})
		}
	}
	return files
}

type File struct {
	Path     string // 文件路径
	FileName string // 文件�?
	Size     int64  // 文件大小
	Type     string // 文件类型
	UpdateAt string // 更新时间 eg:"2025-01-27T16:10:16Z"
	CreateAt string // 创建时间
	Url      string // 下载链接
}

func (f File) GetHash() utils.HashInfo {
	return utils.HashInfo{}
}

func (f File) GetPath() string {
	return f.Path
}

func (f File) GetSize() int64 {
	return f.Size
}

func (f File) GetName() string {
	return f.FileName
}

func (f File) ModTime() time.Time {
	t, _ := time.Parse(time.RFC3339, f.CreateAt)
	return t
}

func (f File) CreateTime() time.Time {
	t, _ := time.Parse(time.RFC3339, f.CreateAt)
	return t
}

func (f File) IsDir() bool {
	return f.Type == "dir"
}

func (f File) GetID() string {
	return f.Url
}
