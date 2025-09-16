package baiduphoto

import (
	"fmt"
	"time"

	"github.com/OpenListTeam/OpenList/v4/pkg/utils"

	"github.com/OpenListTeam/OpenList/v4/internal/model"
)

type TokenErrResp struct {
	ErrorDescription string `json:"error_description"`
	ErrorMsg         string `json:"error"`
}

func (e *TokenErrResp) Error() string {
	return fmt.Sprint(e.ErrorMsg, " : ", e.ErrorDescription)
}

type Erron struct {
	Errno     int `json:"errno"`
	RequestID int `json:"request_id"`
}

// 用户信息
type UInfo struct {
	// uk
	YouaID string `json:"youa_id"`
}

type Page struct {
	HasMore int    `json:"has_more"`
	Cursor  string `json:"cursor"`
}

func (p Page) HasNextPage() bool {
	return p.HasMore == 1
}

type Root = model.Object

type (
	FileListResp struct {
		Page
		List []File `json:"list"`
	}

	File struct {
		Fsid     int64    `json:"fsid"` // 文件ID
		Path     string   `json:"path"` // 文件路径
		Size     int64    `json:"size"`
		Ctime    int64    `json:"ctime"` // 创建时间 s
		Mtime    int64    `json:"mtime"` // 修改时间 s
		Thumburl []string `json:"thumburl"`
		Md5      string   `json:"md5"`
	}
)

func (c *File) GetSize() int64        { return c.Size }
func (c *File) GetName() string       { return getFileName(c.Path) }
func (c *File) CreateTime() time.Time { return time.Unix(c.Ctime, 0) }
func (c *File) ModTime() time.Time    { return time.Unix(c.Mtime, 0) }
func (c *File) IsDir() bool           { return false }
func (c *File) GetID() string         { return "" }
func (c *File) GetPath() string       { return "" }
func (c *File) Thumb() string {
	if len(c.Thumburl) > 0 {
		return c.Thumburl[0]
	}
	return ""
}

func (c *File) GetHash() utils.HashInfo {
	return utils.NewHashInfo(utils.MD5, DecryptMd5(c.Md5))
}

/*相册部分*/
type (
	AlbumListResp struct {
		Page
		List       []Album `json:"list"`
		Reset      int64   `json:"reset"`
		TotalCount int64   `json:"total_count"`
	}

	Album struct {
		AlbumID      string `json:"album_id"`
		Tid          int64  `json:"tid"`
		Title        string `json:"title"`
		JoinTime     int64  `json:"join_time"`
		CreationTime int64  `json:"create_time"`
		Mtime        int64  `json:"mtime"`

		parseTime *time.Time
	}

	AlbumFileListResp struct {
		Page
		List       []AlbumFile `json:"list"`
		Reset      int64       `json:"reset"`
		TotalCount int64       `json:"total_count"`
	}

	AlbumFile struct {
		File
		AlbumID string `json:"album_id"`
		Tid     int64  `json:"tid"`
		Uk      int64  `json:"uk"`
	}
)

func (a *Album) GetHash() utils.HashInfo {
	return utils.HashInfo{}
}

func (a *Album) GetSize() int64        { return 0 }
func (a *Album) GetName() string       { return a.Title }
func (a *Album) CreateTime() time.Time { return time.Unix(a.CreationTime, 0) }
func (a *Album) ModTime() time.Time    { return time.Unix(a.Mtime, 0) }
func (a *Album) IsDir() bool           { return true }
func (a *Album) GetID() string         { return "" }
func (a *Album) GetPath() string       { return "" }

type (
	CopyFileResp struct {
		List []CopyFile `json:"list"`
	}
	CopyFile struct {
		FromFsid  int64  `json:"from_fsid"` // 源ID
		Ctime     int64  `json:"ctime"`
		Fsid      int64  `json:"fsid"` // 目标ID
		Path      string `json:"path"`
		ShootTime int    `json:"shoot_time"`
	}
)

/*上传部分*/
type (
	UploadFile struct {
		FsID           int64  `json:"fs_id"`
		Size           int64  `json:"size"`
		Md5            string `json:"md5"`
		ServerFilename string `json:"server_filename"`
		Path           string `json:"path"`
		Ctime          int64  `json:"ctime"`
		Mtime          int64  `json:"mtime"`
		Isdir          int    `json:"isdir"`
		Category       int    `json:"category"`
		ServerMd5      string `json:"server_md5"`
		ShootTime      int    `json:"shoot_time"`
	}

	CreateFileResp struct {
		Data UploadFile `json:"data"`
	}

	PrecreateResp struct {
		ReturnType int `json:"return_type"` //存在返回2 不存在返�? 已经保存3
		//存在返回
		CreateFileResp

		//不存在返�?
		Path      string `json:"path"`
		UploadID  string `json:"uploadid"`
		BlockList []int  `json:"block_list"`
	}
)

func (f *UploadFile) toFile() *File {
	return &File{
		Fsid:     f.FsID,
		Path:     f.Path,
		Size:     f.Size,
		Ctime:    f.Ctime,
		Mtime:    f.Mtime,
		Thumburl: nil,
	}
}

/* 共享相册部分 */
type InviteResp struct {
	Pdata struct {
		// 邀请码
		InviteCode string `json:"invite_code"`
		// 有效时间
		ExpireTime int    `json:"expire_time"`
		ShareID    string `json:"share_id"`
	} `json:"pdata"`
}

/* 加入相册部分 */
type JoinOrCreateAlbumResp struct {
	AlbumID       string `json:"album_id"`
	AlreadyExists int    `json:"already_exists"`
}
