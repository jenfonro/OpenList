package thunder_browser

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/OpenListTeam/OpenList/v4/drivers/base"
	"github.com/OpenListTeam/OpenList/v4/internal/driver"
	"github.com/OpenListTeam/OpenList/v4/internal/model"
	"github.com/OpenListTeam/OpenList/v4/internal/op"
	streamPkg "github.com/OpenListTeam/OpenList/v4/internal/stream"
	"github.com/OpenListTeam/OpenList/v4/pkg/utils"
	hash_extend "github.com/OpenListTeam/OpenList/v4/pkg/utils/hash"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	"github.com/go-resty/resty/v2"
)

type ThunderBrowser struct {
	*XunLeiBrowserCommon
	model.Storage
	Addition

	identity string
}

func (x *ThunderBrowser) Config() driver.Config {
	return config
}

func (x *ThunderBrowser) GetAddition() driver.Additional {
	return &x.Addition
}

func (x *ThunderBrowser) Init(ctx context.Context) (err error) {

	spaceTokenFunc := func() error {
		// 如果用户未设�?"超级保险�? 密码 则直接返�?
		if x.SafePassword == "" {
			return nil
		}
		// 通过 GetSafeAccessToken 获取
		token, err := x.GetSafeAccessToken(x.SafePassword)
		x.SetSpaceTokenResp(token)
		return err
	}

	// 初始化所需参数
	if x.XunLeiBrowserCommon == nil {
		x.XunLeiBrowserCommon = &XunLeiBrowserCommon{
			Common: &Common{
				client:            base.NewRestyClient(),
				Algorithms:        Algorithms,
				DeviceID:          utils.GetMD5EncodeStr(x.Username + x.Password),
				ClientID:          ClientID,
				ClientSecret:      ClientSecret,
				ClientVersion:     ClientVersion,
				PackageName:       PackageName,
				UserAgent:         BuildCustomUserAgent(utils.GetMD5EncodeStr(x.Username+x.Password), PackageName, SdkVersion, ClientVersion, PackageName),
				DownloadUserAgent: DownloadUserAgent,
				UseVideoUrl:       x.UseVideoUrl,
				UseFluentPlay:     x.UseFluentPlay,
				RemoveWay:         x.Addition.RemoveWay,
				refreshCTokenCk: func(token string) {
					x.CaptchaToken = token
					op.MustSaveDriverStorage(x)
				},
			},
			refreshTokenFunc: func() error {
				// 通过RefreshToken刷新
				token, err := x.RefreshToken(x.TokenResp.RefreshToken)
				if err != nil {
					// 重新登录
					token, err = x.Login(x.Username, x.Password)
					if err != nil {
						x.GetStorage().SetStatus(fmt.Sprintf("%+v", err.Error()))
						op.MustSaveDriverStorage(x)
					}
					// 清空 信任密钥
					x.Addition.CreditKey = ""
				}
				x.SetTokenResp(token)
				return err
			},
		}
	}

	// 自定义验证码token
	ctoekn := strings.TrimSpace(x.CaptchaToken)
	if ctoekn != "" {
		x.SetCaptchaToken(ctoekn)
	}

	if x.Addition.CreditKey != "" {
		x.SetCreditKey(x.Addition.CreditKey)
	}

	if x.Addition.DeviceID != "" {
		x.Common.DeviceID = x.Addition.DeviceID
	} else {
		x.Addition.DeviceID = x.Common.DeviceID
		op.MustSaveDriverStorage(x)
	}

	x.XunLeiBrowserCommon.UseVideoUrl = x.UseVideoUrl
	x.XunLeiBrowserCommon.UseFluentPlay = x.UseFluentPlay
	x.Addition.RootFolderID = x.RootFolderID
	// 防止重复登录
	identity := x.GetIdentity()
	if x.identity != identity || !x.IsLogin() {
		x.identity = identity
		// 登录
		token, err := x.Login(x.Username, x.Password)
		if err != nil {
			return err
		}
		// 清空 信任密钥
		x.Addition.CreditKey = ""
		x.SetTokenResp(token)
	}

	// 获取 spaceToken
	err = spaceTokenFunc()
	if err != nil {
		return err
	}

	return nil
}

func (x *ThunderBrowser) Drop(ctx context.Context) error {
	return nil
}

type ThunderBrowserExpert struct {
	*XunLeiBrowserCommon
	model.Storage
	ExpertAddition

	identity string
}

func (x *ThunderBrowserExpert) Config() driver.Config {
	return configExpert
}

func (x *ThunderBrowserExpert) GetAddition() driver.Additional {
	return &x.ExpertAddition
}

func (x *ThunderBrowserExpert) Init(ctx context.Context) (err error) {

	spaceTokenFunc := func() error {
		// 如果用户未设�?"超级保险�? 密码 则直接返�?
		if x.SafePassword == "" {
			return nil
		}
		// 通过 GetSafeAccessToken 获取
		token, err := x.GetSafeAccessToken(x.SafePassword)
		x.SetSpaceTokenResp(token)
		return err
	}

	// 防止重复登录
	identity := x.GetIdentity()
	if identity != x.identity || !x.IsLogin() {
		x.identity = identity
		x.XunLeiBrowserCommon = &XunLeiBrowserCommon{
			Common: &Common{
				client: base.NewRestyClient(),
				DeviceID: func() string {
					if len(x.DeviceID) != 32 {
						if x.LoginType == "user" {
							return utils.GetMD5EncodeStr(x.Username + x.Password)
						}
						return utils.GetMD5EncodeStr(x.ExpertAddition.RefreshToken)
					}
					return x.DeviceID
				}(),
				ClientID:      x.ClientID,
				ClientSecret:  x.ClientSecret,
				ClientVersion: x.ClientVersion,
				PackageName:   x.PackageName,
				UserAgent: func() string {
					if x.ExpertAddition.UserAgent != "" {
						return x.ExpertAddition.UserAgent
					}
					if x.LoginType == "user" {
						return BuildCustomUserAgent(utils.GetMD5EncodeStr(x.Username+x.Password), x.PackageName, SdkVersion, x.ClientVersion, x.PackageName)
					}
					return BuildCustomUserAgent(utils.GetMD5EncodeStr(x.ExpertAddition.RefreshToken), x.PackageName, SdkVersion, x.ClientVersion, x.PackageName)
				}(),
				DownloadUserAgent: func() string {
					if x.ExpertAddition.DownloadUserAgent != "" {
						return x.ExpertAddition.DownloadUserAgent
					}
					return DownloadUserAgent
				}(),
				UseVideoUrl:   x.UseVideoUrl,
				UseFluentPlay: x.UseFluentPlay,
				RemoveWay:     x.ExpertAddition.RemoveWay,
				refreshCTokenCk: func(token string) {
					x.CaptchaToken = token
					op.MustSaveDriverStorage(x)
				},
			},
		}

		if x.ExpertAddition.CaptchaToken != "" {
			x.SetCaptchaToken(x.ExpertAddition.CaptchaToken)
			op.MustSaveDriverStorage(x)
		}
		if x.ExpertAddition.CreditKey != "" {
			x.SetCreditKey(x.ExpertAddition.CreditKey)
		}

		if x.ExpertAddition.DeviceID != "" {
			x.Common.DeviceID = x.ExpertAddition.DeviceID
		} else {
			x.ExpertAddition.DeviceID = x.Common.DeviceID
			op.MustSaveDriverStorage(x)
		}
		if x.Common.UserAgent != "" {
			x.ExpertAddition.UserAgent = x.Common.UserAgent
			op.MustSaveDriverStorage(x)
		}
		if x.Common.DownloadUserAgent != "" {
			x.ExpertAddition.DownloadUserAgent = x.Common.DownloadUserAgent
			op.MustSaveDriverStorage(x)
		}
		x.XunLeiBrowserCommon.UseVideoUrl = x.UseVideoUrl
		x.XunLeiBrowserCommon.UseFluentPlay = x.UseFluentPlay
		x.ExpertAddition.RootFolderID = x.RootFolderID
		// 签名方法
		if x.SignType == "captcha_sign" {
			x.Common.Timestamp = x.Timestamp
			x.Common.CaptchaSign = x.CaptchaSign
		} else {
			x.Common.Algorithms = strings.Split(x.Algorithms, ",")
		}

		// 登录方式
		if x.LoginType == "refresh_token" {
			// 通过RefreshToken登录
			token, err := x.XunLeiBrowserCommon.RefreshToken(x.ExpertAddition.RefreshToken)
			if err != nil {
				return err
			}
			x.SetTokenResp(token)

			// 刷新token方法
			x.SetRefreshTokenFunc(func() error {
				token, err := x.XunLeiBrowserCommon.RefreshToken(x.TokenResp.RefreshToken)
				if err != nil {
					x.GetStorage().SetStatus(fmt.Sprintf("%+v", err.Error()))
				}
				x.SetTokenResp(token)
				op.MustSaveDriverStorage(x)
				return err
			})

			err = spaceTokenFunc()
			if err != nil {
				return err
			}

		} else {
			// 通过用户密码登录
			token, err := x.Login(x.Username, x.Password)
			if err != nil {
				return err
			}
			// 清空 信任密钥
			x.ExpertAddition.CreditKey = ""
			x.SetTokenResp(token)
			x.SetRefreshTokenFunc(func() error {
				token, err := x.XunLeiBrowserCommon.RefreshToken(x.TokenResp.RefreshToken)
				if err != nil {
					token, err = x.Login(x.Username, x.Password)
					if err != nil {
						x.GetStorage().SetStatus(fmt.Sprintf("%+v", err.Error()))
					}
					// 清空 信任密钥
					x.ExpertAddition.CreditKey = ""
				}
				x.SetTokenResp(token)
				op.MustSaveDriverStorage(x)
				return err
			})

			err = spaceTokenFunc()
			if err != nil {
				return err
			}
		}
	} else {
		// 仅修改验证码token
		if x.CaptchaToken != "" {
			x.SetCaptchaToken(x.CaptchaToken)
		}

		err = spaceTokenFunc()
		if err != nil {
			return err
		}

		x.XunLeiBrowserCommon.UserAgent = x.UserAgent
		x.XunLeiBrowserCommon.DownloadUserAgent = x.DownloadUserAgent
		x.XunLeiBrowserCommon.UseVideoUrl = x.UseVideoUrl
		x.XunLeiBrowserCommon.UseFluentPlay = x.UseFluentPlay
		x.ExpertAddition.RootFolderID = x.RootFolderID
	}

	return nil
}

func (x *ThunderBrowserExpert) Drop(ctx context.Context) error {
	return nil
}

func (x *ThunderBrowserExpert) SetTokenResp(token *TokenResp) {
	x.XunLeiBrowserCommon.SetTokenResp(token)
	if token != nil {
		x.ExpertAddition.RefreshToken = token.RefreshToken
	}
}

type XunLeiBrowserCommon struct {
	*Common
	*TokenResp     // 登录信息
	*CoreLoginResp // core登录信息

	refreshTokenFunc func() error
}

func (xc *XunLeiBrowserCommon) List(ctx context.Context, dir model.Obj, args model.ListArgs) ([]model.Obj, error) {
	return xc.getFiles(ctx, dir, args.ReqPath)
}

func (xc *XunLeiBrowserCommon) Link(ctx context.Context, file model.Obj, args model.LinkArgs) (*model.Link, error) {
	var lFile Files

	params := map[string]string{
		"_magic":         "2021",
		"space":          file.(*Files).GetSpace(),
		"thumbnail_size": "SIZE_LARGE",
		"with":           "url",
	}

	_, err := xc.Request(FILE_API_URL+"/{fileID}", http.MethodGet, func(r *resty.Request) {
		r.SetContext(ctx)
		r.SetPathParam("fileID", file.GetID())
		r.SetQueryParams(params)
		//r.SetQueryParam("space", "")
	}, &lFile)
	if err != nil {
		return nil, err
	}
	link := &model.Link{
		URL: lFile.WebContentLink,
		Header: http.Header{
			"User-Agent": {xc.DownloadUserAgent},
		},
	}

	if xc.UseVideoUrl {
		for _, media := range lFile.Medias {
			if media.Link.URL != "" {
				link.URL = media.Link.URL
				break
			}
		}
	}
	return link, nil
}

func (xc *XunLeiBrowserCommon) MakeDir(ctx context.Context, parentDir model.Obj, dirName string) error {
	js := base.Json{
		"kind":      FOLDER,
		"name":      dirName,
		"parent_id": parentDir.GetID(),
		"space":     parentDir.(*Files).GetSpace(),
	}

	_, err := xc.Request(FILE_API_URL, http.MethodPost, func(r *resty.Request) {
		r.SetContext(ctx)
		r.SetBody(&js)
	}, nil)
	return err
}

func (xc *XunLeiBrowserCommon) Move(ctx context.Context, srcObj, dstDir model.Obj) error {

	params := map[string]string{
		"_from": srcObj.(*Files).GetSpace(),
	}
	js := base.Json{
		"to":    base.Json{"parent_id": dstDir.GetID(), "space": dstDir.(*Files).GetSpace()},
		"space": srcObj.(*Files).GetSpace(),
		"ids":   []string{srcObj.GetID()},
	}

	_, err := xc.Request(FILE_API_URL+":batchMove", http.MethodPost, func(r *resty.Request) {
		r.SetContext(ctx)
		r.SetBody(&js)
		r.SetQueryParams(params)
	}, nil)
	return err
}

func (xc *XunLeiBrowserCommon) Rename(ctx context.Context, srcObj model.Obj, newName string) error {

	params := map[string]string{
		"space": srcObj.(*Files).GetSpace(),
	}

	_, err := xc.Request(FILE_API_URL+"/{fileID}", http.MethodPatch, func(r *resty.Request) {
		r.SetContext(ctx)
		r.SetPathParam("fileID", srcObj.GetID())
		r.SetBody(&base.Json{"name": newName})
		r.SetQueryParams(params)
	}, nil)
	return err
}

func (xc *XunLeiBrowserCommon) Copy(ctx context.Context, srcObj, dstDir model.Obj) error {

	params := map[string]string{
		"_from": srcObj.(*Files).GetSpace(),
	}
	js := base.Json{
		"to":    base.Json{"parent_id": dstDir.GetID(), "space": dstDir.(*Files).GetSpace()},
		"space": srcObj.(*Files).GetSpace(),
		"ids":   []string{srcObj.GetID()},
	}

	_, err := xc.Request(FILE_API_URL+":batchCopy", http.MethodPost, func(r *resty.Request) {
		r.SetContext(ctx)
		r.SetBody(&js)
		r.SetQueryParams(params)
	}, nil)
	return err
}

func (xc *XunLeiBrowserCommon) Remove(ctx context.Context, obj model.Obj) error {

	js := base.Json{
		"ids":   []string{obj.GetID()},
		"space": obj.(*Files).GetSpace(),
	}
	// 先判断是否是特殊情况
	if obj.(*Files).GetSpace() == ThunderDriveSpace {
		_, err := xc.Request(FILE_API_URL+"/{fileID}/trash", http.MethodPatch, func(r *resty.Request) {
			r.SetContext(ctx)
			r.SetPathParam("fileID", obj.GetID())
			r.SetBody("{}")
		}, nil)
		return err
	} else if obj.(*Files).GetSpace() == ThunderBrowserDriveSafeSpace || obj.(*Files).GetSpace() == ThunderDriveSafeSpace {
		_, err := xc.Request(FILE_API_URL+":batchDelete", http.MethodPost, func(r *resty.Request) {
			r.SetContext(ctx)
			r.SetBody(&js)
		}, nil)
		return err
	}

	// 根据用户选择的删除方式进行删�?
	if xc.RemoveWay == "delete" {
		_, err := xc.Request(FILE_API_URL+":batchDelete", http.MethodPost, func(r *resty.Request) {
			r.SetContext(ctx)
			r.SetBody(&js)
		}, nil)
		return err
	} else {
		_, err := xc.Request(FILE_API_URL+":batchTrash", http.MethodPost, func(r *resty.Request) {
			r.SetContext(ctx)
			r.SetBody(&js)
		}, nil)
		return err
	}
}

func (xc *XunLeiBrowserCommon) Put(ctx context.Context, dstDir model.Obj, stream model.FileStreamer, up driver.UpdateProgress) error {
	gcid := stream.GetHash().GetHash(hash_extend.GCID)
	var err error
	if len(gcid) < hash_extend.GCID.Width {
		_, gcid, err = streamPkg.CacheFullAndHash(stream, &up, hash_extend.GCID, stream.GetSize())
		if err != nil {
			return err
		}
	}

	js := base.Json{
		"kind":        FILE,
		"parent_id":   dstDir.GetID(),
		"name":        stream.GetName(),
		"size":        stream.GetSize(),
		"hash":        gcid,
		"upload_type": UPLOAD_TYPE_RESUMABLE,
		"space":       dstDir.(*Files).GetSpace(),
	}

	var resp UploadTaskResponse
	_, err = xc.Request(FILE_API_URL, http.MethodPost, func(r *resty.Request) {
		r.SetContext(ctx)
		r.SetBody(&js)
	}, &resp)
	if err != nil {
		return err
	}

	param := resp.Resumable.Params
	if resp.UploadType == UPLOAD_TYPE_RESUMABLE {
		param.Endpoint = strings.TrimLeft(param.Endpoint, param.Bucket+".")
		s, err := session.NewSession(&aws.Config{
			Credentials: credentials.NewStaticCredentials(param.AccessKeyID, param.AccessKeySecret, param.SecurityToken),
			Region:      aws.String("xunlei"),
			Endpoint:    aws.String(param.Endpoint),
		})
		if err != nil {
			return err
		}
		uploader := s3manager.NewUploader(s)
		if stream.GetSize() > s3manager.MaxUploadParts*s3manager.DefaultUploadPartSize {
			uploader.PartSize = stream.GetSize() / (s3manager.MaxUploadParts - 1)
		}
		_, err = uploader.UploadWithContext(ctx, &s3manager.UploadInput{
			Bucket:  aws.String(param.Bucket),
			Key:     aws.String(param.Key),
			Expires: aws.Time(param.Expiration),
			Body:    driver.NewLimitedUploadStream(ctx, io.TeeReader(stream, driver.NewProgress(stream.GetSize(), up))),
		})
		return err
	}
	return nil
}

func (xc *XunLeiBrowserCommon) getFiles(ctx context.Context, dir model.Obj, path string) ([]model.Obj, error) {
	files := make([]model.Obj, 0)
	var pageToken string
	for {
		var fileList FileList
		folderSpace := ""
		switch dirF := dir.(type) {
		case *Files:
			folderSpace = dirF.GetSpace()
		default:
			// 处理 根目录的情况
			//folderSpace = ThunderBrowserDriveSpace
			folderSpace = ThunderDriveSpace // 迅雷浏览器已经合并到迅雷云盘，因此变更根目录
		}
		params := map[string]string{
			"parent_id":      dir.GetID(),
			"page_token":     pageToken,
			"space":          folderSpace,
			"filters":        `{"trashed":{"eq":false}}`,
			"with":           "url",
			"with_audit":     "true",
			"thumbnail_size": "SIZE_LARGE",
		}

		_, err := xc.Request(FILE_API_URL, http.MethodGet, func(r *resty.Request) {
			r.SetContext(ctx)
			r.SetQueryParams(params)
		}, &fileList)
		if err != nil {
			return nil, err
		}

		for i := range fileList.Files {
			// 解决 "迅雷云盘" 重复出现问题————迅雷后端发送错�?
			if fileList.Files[i].FolderType == ThunderDriveFolderType && fileList.Files[i].ID == "" && fileList.Files[i].Space == "" && dir.GetID() != "" {
				continue
			}
			files = append(files, &fileList.Files[i])
		}

		if fileList.NextPageToken == "" {
			break
		}
		pageToken = fileList.NextPageToken
	}
	return files, nil
}

// SetRefreshTokenFunc 设置刷新Token的方�?
func (xc *XunLeiBrowserCommon) SetRefreshTokenFunc(fn func() error) {
	xc.refreshTokenFunc = fn
}

// SetTokenResp 设置Token
func (xc *XunLeiBrowserCommon) SetTokenResp(tr *TokenResp) {
	xc.TokenResp = tr
}

// SetCoreTokenResp 设置CoreToken
func (xc *XunLeiBrowserCommon) SetCoreTokenResp(tr *CoreLoginResp) {
	xc.CoreLoginResp = tr
}

// SetSpaceTokenResp 设置Token
func (xc *XunLeiBrowserCommon) SetSpaceTokenResp(spaceToken string) {
	xc.TokenResp.Token = spaceToken
}

// Request 携带Authorization和CaptchaToken的请�?
func (xc *XunLeiBrowserCommon) Request(url string, method string, callback base.ReqCallback, resp interface{}) ([]byte, error) {
	data, err := xc.Common.Request(url, method, func(req *resty.Request) {
		req.SetHeaders(map[string]string{
			"Authorization":         xc.GetToken(),
			"X-Captcha-Token":       xc.GetCaptchaToken(),
			"X-Space-Authorization": xc.GetSpaceToken(),
		})
		if callback != nil {
			callback(req)
		}
	}, resp)

	errResp, ok := err.(*ErrResp)
	if !ok {
		return nil, err
	}

	switch errResp.ErrorCode {
	case 0:
		return data, nil
	case 4122, 4121, 10, 16:
		if xc.refreshTokenFunc != nil {
			if err = xc.refreshTokenFunc(); err == nil {
				break
			}
		}
		return nil, err
	case 9:
		// space_token 获取失败
		if errResp.ErrorMsg == "space_token_invalid" {
			if token, err := xc.GetSafeAccessToken(xc.Token); err != nil {
				return nil, err
			} else {
				xc.SetSpaceTokenResp(token)
			}

		}
		if errResp.ErrorMsg == "captcha_invalid" {
			// 验证码token过期
			if err = xc.RefreshCaptchaTokenAtLogin(GetAction(method, url), xc.TokenResp.UserID); err != nil {
				return nil, err
			}
		}

		return nil, errors.New(errResp.ErrorMsg)
	default:
		// 处理未捕获到的验证码错误
		if errResp.ErrorMsg == "captcha_invalid" {
			// 验证码token过期
			if err = xc.RefreshCaptchaTokenAtLogin(GetAction(method, url), xc.TokenResp.UserID); err != nil {
				return nil, err
			}
		}

		return nil, err
	}

	return xc.Request(url, method, callback, resp)
}

// RefreshToken 刷新Token
func (xc *XunLeiBrowserCommon) RefreshToken(refreshToken string) (*TokenResp, error) {
	var resp TokenResp
	_, err := xc.Common.Request(XLUSER_API_URL+"/auth/token", http.MethodPost, func(req *resty.Request) {
		req.SetBody(&base.Json{
			"grant_type":    "refresh_token",
			"refresh_token": refreshToken,
			"client_id":     xc.ClientID,
			"client_secret": xc.ClientSecret,
		})
	}, &resp)
	if err != nil {
		return nil, err
	}

	if resp.RefreshToken == "" {
		return nil, errors.New("refresh token is empty")
	}
	return &resp, nil
}

// GetSafeAccessToken 获取 超级保险�?AccessToken
func (xc *XunLeiBrowserCommon) GetSafeAccessToken(safePassword string) (string, error) {
	var resp TokenResp
	_, err := xc.Request(XLUSER_API_URL+"/password/check", http.MethodPost, func(req *resty.Request) {
		req.SetBody(&base.Json{
			"scene":    "box",
			"password": EncryptPassword(safePassword),
		})
	}, &resp)
	if err != nil {
		return "", err
	}

	if resp.Token == "" {
		return "", errors.New("SafePassword is incorrect ")
	}
	return resp.Token, nil
}

// Login 登录
func (xc *XunLeiBrowserCommon) Login(username, password string) (*TokenResp, error) {
	//v3 login拿到 sessionID
	sessionID, err := xc.CoreLogin(username, password)
	if err != nil {
		return nil, err
	}
	//v1 login拿到令牌
	url := XLUSER_API_URL + "/auth/signin/token"
	if err = xc.RefreshCaptchaTokenInLogin(GetAction(http.MethodPost, url), username); err != nil {
		return nil, err
	}

	var resp TokenResp
	_, err = xc.Common.Request(url, http.MethodPost, func(req *resty.Request) {
		req.SetPathParam("client_id", xc.ClientID)
		req.SetBody(&SignInRequest{
			ClientID:     xc.ClientID,
			ClientSecret: xc.ClientSecret,
			Provider:     SignProvider,
			SigninToken:  sessionID,
		})
	}, &resp)
	if err != nil {
		return nil, err
	}
	return &resp, nil
}

func (xc *XunLeiBrowserCommon) IsLogin() bool {
	if xc.TokenResp == nil {
		return false
	}
	_, err := xc.Request(XLUSER_API_URL+"/user/me", http.MethodGet, nil, nil)
	return err == nil
}

// OfflineDownload 离线下载文件
func (xc *XunLeiBrowserCommon) OfflineDownload(ctx context.Context, fileUrl string, parentDir model.Obj, fileName string) (*OfflineTask, error) {
	var resp OfflineDownloadResp

	body := base.Json{}

	from := "cloudadd/"

	if xc.UseFluentPlay {
		body = base.Json{
			"kind": FILE,
			"name": fileName,
			// 流畅播接�?强制将文件放�?"SPACE_FAVORITE" 文件�?
			//"parent_id":   parentDir.GetID(),
			"upload_type": UPLOAD_TYPE_URL,
			"url": base.Json{
				"url": fileUrl,
				//"files": []string{"0"}, // 0 表示只下载第一个文�?
			},
			"params": base.Json{
				"cookie":      "null",
				"web_title":   "",
				"lastSession": "",
				"flags":       "9",
				"scene":       "smart_spot_panel",
				"referer":     "https://x.xunlei.com",
				"dedup_index": "0",
			},
			"need_dedup":  true,
			"folder_type": "FAVORITE",
			"space":       ThunderBrowserDriveFluentPlayFolderType,
		}

		from = "FLUENT_PLAY/sniff_ball/fluent_play/SPACE_FAVORITE"
	} else {
		body = base.Json{
			"kind":        FILE,
			"name":        fileName,
			"parent_id":   parentDir.GetID(),
			"upload_type": UPLOAD_TYPE_URL,
			"url": base.Json{
				"url": fileUrl,
			},
		}

		if files, ok := parentDir.(*Files); ok {
			body["space"] = files.GetSpace()
		} else {
			// 如果不是 Files 类型，则默认使用 ThunderDriveSpace
			body["space"] = ThunderDriveSpace
		}
	}

	_, err := xc.Request(FILE_API_URL, http.MethodPost, func(r *resty.Request) {
		r.SetContext(ctx)
		r.SetQueryParam("_from", from)
		r.SetBody(&body)
	}, &resp)

	if err != nil {
		return nil, err
	}

	return &resp.Task, err
}

// OfflineList 获取离线下载任务列表
func (xc *XunLeiBrowserCommon) OfflineList(ctx context.Context, nextPageToken string) ([]OfflineTask, error) {
	res := make([]OfflineTask, 0)

	var resp OfflineListResp
	_, err := xc.Request(TASK_API_URL, http.MethodGet, func(req *resty.Request) {
		req.SetContext(ctx).
			SetQueryParams(map[string]string{
				"type":       "offline",
				"limit":      "10000",
				"page_token": nextPageToken,
				"space":      "default/*",
			})
	}, &resp)

	if err != nil {
		return nil, fmt.Errorf("failed to get offline list: %w", err)
	}
	res = append(res, resp.Tasks...)

	return res, nil
}

func (xc *XunLeiBrowserCommon) DeleteOfflineTasks(ctx context.Context, taskIDs []string) error {
	queryParams := map[string]string{
		"task_ids": strings.Join(taskIDs, ","),
		"_t":       fmt.Sprintf("%d", time.Now().UnixMilli()),
	}
	if xc.UseFluentPlay {
		queryParams["space"] = ThunderBrowserDriveFluentPlayFolderType
	}

	_, err := xc.Request(TASK_API_URL, http.MethodDelete, func(req *resty.Request) {
		req.SetContext(ctx).
			SetQueryParams(queryParams)
	}, nil)
	if err != nil {
		return fmt.Errorf("failed to delete tasks %v: %w", taskIDs, err)
	}

	return nil
}

func (xc *XunLeiBrowserCommon) CoreLogin(username string, password string) (sessionID string, err error) {
	url := XLUSER_API_BASE_URL + "/xluser.core.login/v3/login"
	var resp CoreLoginResp
	res, err := xc.Common.Request(url, http.MethodPost, func(req *resty.Request) {
		req.SetHeader("User-Agent", "android-ok-http-client/xl-acc-sdk/version-5.0.9.509300")
		req.SetBody(&CoreLoginRequest{
			ProtocolVersion: "301",
			SequenceNo:      "1000010",
			PlatformVersion: "10",
			IsCompressed:    "0",
			Appid:           APPID,
			ClientVersion:   xc.Common.ClientVersion,
			PeerID:          "00000000000000000000000000000000",
			AppName:         "ANDROID-com.xunlei.browser",
			SdkVersion:      "509300",
			Devicesign:      generateDeviceSign(xc.DeviceID, xc.PackageName),
			NetWorkType:     "WIFI",
			ProviderName:    "NONE",
			DeviceModel:     "M2004J7AC",
			DeviceName:      "Xiaomi_M2004j7ac",
			OSVersion:       "12",
			Creditkey:       xc.GetCreditKey(),
			Hl:              "zh-CN",
			UserName:        username,
			PassWord:        password,
			VerifyKey:       "",
			VerifyCode:      "",
			IsMd5Pwd:        "0",
		})
	}, nil)
	if err != nil {
		return "", err
	}

	if err = utils.Json.Unmarshal(res, &resp); err != nil {
		return "", err
	}

	xc.SetCoreTokenResp(&resp)

	sessionID = resp.SessionID

	return sessionID, nil
}
