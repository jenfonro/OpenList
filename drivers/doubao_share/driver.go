package doubao_share

import (
	"context"
	"errors"
	"net/http"

	"github.com/OpenListTeam/OpenList/v4/drivers/base"
	"github.com/OpenListTeam/OpenList/v4/internal/driver"
	"github.com/OpenListTeam/OpenList/v4/internal/errs"
	"github.com/OpenListTeam/OpenList/v4/internal/model"
	"github.com/OpenListTeam/OpenList/v4/pkg/utils"
	"github.com/go-resty/resty/v2"
)

type DoubaoShare struct {
	model.Storage
	Addition
	RootFiles []RootFileList
}

func (d *DoubaoShare) Config() driver.Config {
	return config
}

func (d *DoubaoShare) GetAddition() driver.Additional {
	return &d.Addition
}

func (d *DoubaoShare) Init(ctx context.Context) error {
	// 初始�?虚拟分享列表
	if err := d.initShareList(); err != nil {
		return err
	}

	return nil
}

func (d *DoubaoShare) Drop(ctx context.Context) error {
	return nil
}

func (d *DoubaoShare) List(ctx context.Context, dir model.Obj, args model.ListArgs) ([]model.Obj, error) {
	// 检查是否为根目�?
	if dir.GetID() == "" && dir.GetPath() == "/" {
		return d.listRootDirectory(ctx)
	}

	// 非根目录，处理不同情�?
	if fo, ok := dir.(*FileObject); ok {
		if fo.ShareID == "" {
			// 虚拟目录，需要列出子目录
			return d.listVirtualDirectoryContent(dir)
		} else {
			// 具有分享ID的目录，获取此分享下的文�?
			shareId, relativePath, err := d._findShareAndPath(dir)
			if err != nil {
				return nil, err
			}
			return d.getFilesInPath(ctx, shareId, dir.GetID(), relativePath)
		}
	}

	// 使用通用方法
	shareId, relativePath, err := d._findShareAndPath(dir)
	if err != nil {
		return nil, err
	}

	// 获取指定路径下的文件
	return d.getFilesInPath(ctx, shareId, dir.GetID(), relativePath)
}

func (d *DoubaoShare) Link(ctx context.Context, file model.Obj, args model.LinkArgs) (*model.Link, error) {
	var downloadUrl string

	if u, ok := file.(*FileObject); ok {
		switch u.NodeType {
		case VideoType, AudioType:
			var r GetVideoFileUrlResp
			_, err := d.request("/samantha/media/get_play_info", http.MethodPost, func(req *resty.Request) {
				req.SetBody(base.Json{
					"key":      u.Key,
					"share_id": u.ShareID,
					"node_id":  file.GetID(),
				})
			}, &r)
			if err != nil {
				return nil, err
			}

			downloadUrl = r.Data.OriginalMediaInfo.MainURL
		default:
			var r GetFileUrlResp
			_, err := d.request("/alice/message/get_file_url", http.MethodPost, func(req *resty.Request) {
				req.SetBody(base.Json{
					"uris": []string{u.Key},
					"type": FileNodeType[u.NodeType],
				})
			}, &r)
			if err != nil {
				return nil, err
			}

			downloadUrl = r.Data.FileUrls[0].MainURL
		}

		// 生成标准的Content-Disposition
		contentDisposition := utils.GenerateContentDisposition(u.Name)

		return &model.Link{
			URL: downloadUrl,
			Header: http.Header{
				"User-Agent":          []string{UserAgent},
				"Content-Disposition": []string{contentDisposition},
			},
		}, nil
	}

	return nil, errors.New("can't convert obj to URL")
}

func (d *DoubaoShare) MakeDir(ctx context.Context, parentDir model.Obj, dirName string) (model.Obj, error) {
	// TODO create folder, optional
	return nil, errs.NotImplement
}

func (d *DoubaoShare) Move(ctx context.Context, srcObj, dstDir model.Obj) (model.Obj, error) {
	// TODO move obj, optional
	return nil, errs.NotImplement
}

func (d *DoubaoShare) Rename(ctx context.Context, srcObj model.Obj, newName string) (model.Obj, error) {
	// TODO rename obj, optional
	return nil, errs.NotImplement
}

func (d *DoubaoShare) Copy(ctx context.Context, srcObj, dstDir model.Obj) (model.Obj, error) {
	// TODO copy obj, optional
	return nil, errs.NotImplement
}

func (d *DoubaoShare) Remove(ctx context.Context, obj model.Obj) error {
	// TODO remove obj, optional
	return errs.NotImplement
}

func (d *DoubaoShare) Put(ctx context.Context, dstDir model.Obj, file model.FileStreamer, up driver.UpdateProgress) (model.Obj, error) {
	// TODO upload file, optional
	return nil, errs.NotImplement
}

func (d *DoubaoShare) GetArchiveMeta(ctx context.Context, obj model.Obj, args model.ArchiveArgs) (model.ArchiveMeta, error) {
	// TODO get archive file meta-info, return errs.NotImplement to use an internal archive tool, optional
	return nil, errs.NotImplement
}

func (d *DoubaoShare) ListArchive(ctx context.Context, obj model.Obj, args model.ArchiveInnerArgs) ([]model.Obj, error) {
	// TODO list args.InnerPath in the archive obj, return errs.NotImplement to use an internal archive tool, optional
	return nil, errs.NotImplement
}

func (d *DoubaoShare) Extract(ctx context.Context, obj model.Obj, args model.ArchiveInnerArgs) (*model.Link, error) {
	// TODO return link of file args.InnerPath in the archive obj, return errs.NotImplement to use an internal archive tool, optional
	return nil, errs.NotImplement
}

func (d *DoubaoShare) ArchiveDecompress(ctx context.Context, srcObj, dstDir model.Obj, args model.ArchiveDecompressArgs) ([]model.Obj, error) {
	// TODO extract args.InnerPath path in the archive srcObj to the dstDir location, optional
	// a folder with the same name as the archive file needs to be created to store the extracted results if args.PutIntoNewDir
	// return errs.NotImplement to use an internal archive tool
	return nil, errs.NotImplement
}

//func (d *DoubaoShare) Other(ctx context.Context, args model.OtherArgs) (interface{}, error) {
//	return nil, errs.NotSupport
//}

var _ driver.Driver = (*DoubaoShare)(nil)
