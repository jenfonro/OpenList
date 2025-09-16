package thunderx

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/OpenListTeam/OpenList/v4/drivers/base"
	"github.com/OpenListTeam/OpenList/v4/internal/driver"
	"github.com/OpenListTeam/OpenList/v4/internal/errs"
	"github.com/OpenListTeam/OpenList/v4/internal/model"
	"github.com/OpenListTeam/OpenList/v4/internal/op"
	"github.com/OpenListTeam/OpenList/v4/internal/stream"
	"github.com/OpenListTeam/OpenList/v4/pkg/utils"
	hash_extend "github.com/OpenListTeam/OpenList/v4/pkg/utils/hash"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	"github.com/go-resty/resty/v2"
)

type ThunderX struct {
	*XunLeiXCommon
	model.Storage
	Addition

	identity string
}

func (x *ThunderX) Config() driver.Config {
	return config
}

func (x *ThunderX) GetAddition() driver.Additional {
	return &x.Addition
}

func (x *ThunderX) Init(ctx context.Context) (err error) {
	// 初始化所需参数
	if x.XunLeiXCommon == nil {
		x.XunLeiXCommon = &XunLeiXCommon{
			Common: &Common{
				client:            base.NewRestyClient(),
				Algorithms:        Algorithms,
				DeviceID:          utils.GetMD5EncodeStr(x.Username + x.Password),
				ClientID:          ClientID,
				ClientSecret:      ClientSecret,
				ClientVersion:     ClientVersion,
				PackageName:       PackageName,
				UserAgent:         BuildCustomUserAgent(utils.GetMD5EncodeStr(x.Username+x.Password), ClientID, PackageName, SdkVersion, ClientVersion, PackageName, ""),
				DownloadUserAgent: DownloadUserAgent,
				UseVideoUrl:       x.UseVideoUrl,

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
						if token.UserID != "" {
							x.SetUserID(token.UserID)
							x.UserAgent = BuildCustomUserAgent(utils.GetMD5EncodeStr(x.Username+x.Password), ClientID, PackageName, SdkVersion, ClientVersion, PackageName, token.UserID)
						}
						op.MustSaveDriverStorage(x)
					}
				}
				x.SetTokenResp(token)
				return err
			},
		}
	}

	// 自定义验证码token
	ctoken := strings.TrimSpace(x.CaptchaToken)
	if ctoken != "" {
		x.SetCaptchaToken(ctoken)
	}
	if x.DeviceID == "" {
		x.SetDeviceID(utils.GetMD5EncodeStr(x.Username + x.Password))
	}

	x.XunLeiXCommon.UseVideoUrl = x.UseVideoUrl
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
		x.SetTokenResp(token)
		if token.UserID != "" {
			x.SetUserID(token.UserID)
			x.UserAgent = BuildCustomUserAgent(x.DeviceID, ClientID, PackageName, SdkVersion, ClientVersion, PackageName, token.UserID)
		}
	}
	return nil
}

func (x *ThunderX) Drop(ctx context.Context) error {
	return nil
}

type ThunderXExpert struct {
	*XunLeiXCommon
	model.Storage
	ExpertAddition

	identity string
}

func (x *ThunderXExpert) Config() driver.Config {
	return configExpert
}

func (x *ThunderXExpert) GetAddition() driver.Additional {
	return &x.ExpertAddition
}

func (x *ThunderXExpert) Init(ctx context.Context) (err error) {
	// 防止重复登录
	identity := x.GetIdentity()
	if identity != x.identity || !x.IsLogin() {
		x.identity = identity
		x.XunLeiXCommon = &XunLeiXCommon{
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
						return BuildCustomUserAgent(utils.GetMD5EncodeStr(x.Username+x.Password), ClientID, PackageName, SdkVersion, ClientVersion, PackageName, "")
					}
					return BuildCustomUserAgent(utils.GetMD5EncodeStr(x.ExpertAddition.RefreshToken), ClientID, PackageName, SdkVersion, ClientVersion, PackageName, "")
				}(),
				DownloadUserAgent: func() string {
					if x.ExpertAddition.DownloadUserAgent != "" {
						return x.ExpertAddition.DownloadUserAgent
					}
					return DownloadUserAgent
				}(),
				UseVideoUrl: x.UseVideoUrl,
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
		if x.Common.DeviceID != "" {
			x.ExpertAddition.DeviceID = x.Common.DeviceID
			op.MustSaveDriverStorage(x)
		}
		if x.Common.DownloadUserAgent != "" {
			x.ExpertAddition.DownloadUserAgent = x.Common.DownloadUserAgent
			op.MustSaveDriverStorage(x)
		}
		x.XunLeiXCommon.UseVideoUrl = x.UseVideoUrl
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
			token, err := x.XunLeiXCommon.RefreshToken(x.ExpertAddition.RefreshToken)
			if err != nil {
				return err
			}
			x.SetTokenResp(token)
			// 刷新token方法
			x.SetRefreshTokenFunc(func() error {
				token, err := x.XunLeiXCommon.RefreshToken(x.TokenResp.RefreshToken)
				if err != nil {
					x.GetStorage().SetStatus(fmt.Sprintf("%+v", err.Error()))
				}
				x.SetTokenResp(token)
				op.MustSaveDriverStorage(x)
				return err
			})
		} else {
			// 通过用户密码登录
			token, err := x.Login(x.Username, x.Password)
			if err != nil {
				return err
			}
			x.SetTokenResp(token)
			x.SetRefreshTokenFunc(func() error {
				token, err := x.XunLeiXCommon.RefreshToken(x.TokenResp.RefreshToken)
				if err != nil {
					token, err = x.Login(x.Username, x.Password)
					if err != nil {
						x.GetStorage().SetStatus(fmt.Sprintf("%+v", err.Error()))
					}
				}
				x.SetTokenResp(token)
				op.MustSaveDriverStorage(x)
				return err
			})
		}
		// 更新 UserAgent
		if x.TokenResp.UserID != "" {
			x.ExpertAddition.UserAgent = BuildCustomUserAgent(x.ExpertAddition.DeviceID, ClientID, PackageName, SdkVersion, ClientVersion, PackageName, x.TokenResp.UserID)
			x.SetUserAgent(x.ExpertAddition.UserAgent)
			op.MustSaveDriverStorage(x)
		}
	} else {
		// 仅修改验证码token
		if x.CaptchaToken != "" {
			x.SetCaptchaToken(x.CaptchaToken)
		}
		x.XunLeiXCommon.UserAgent = x.ExpertAddition.UserAgent
		x.XunLeiXCommon.DownloadUserAgent = x.ExpertAddition.UserAgent
		x.XunLeiXCommon.UseVideoUrl = x.UseVideoUrl
		x.ExpertAddition.RootFolderID = x.RootFolderID
	}
	return nil
}

func (x *ThunderXExpert) Drop(ctx context.Context) error {
	return nil
}

func (x *ThunderXExpert) SetTokenResp(token *TokenResp) {
	x.XunLeiXCommon.SetTokenResp(token)
	if token != nil {
		x.ExpertAddition.RefreshToken = token.RefreshToken
	}
}

type XunLeiXCommon struct {
	*Common
	*TokenResp // 登录信息

	refreshTokenFunc func() error
}

func (xc *XunLeiXCommon) List(ctx context.Context, dir model.Obj, args model.ListArgs) ([]model.Obj, error) {
	return xc.getFiles(ctx, dir.GetID())
}

func (xc *XunLeiXCommon) Link(ctx context.Context, file model.Obj, args model.LinkArgs) (*model.Link, error) {
	var lFile Files
	_, err := xc.Request(FILE_API_URL+"/{fileID}", http.MethodGet, func(r *resty.Request) {
		r.SetContext(ctx)
		r.SetPathParam("fileID", file.GetID())
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

	/*
		strs := regexp.MustCompile(`e=([0-9]*)`).FindStringSubmatch(lFile.WebContentLink)
		if len(strs) == 2 {
			timestamp, err := strconv.ParseInt(strs[1], 10, 64)
			if err == nil {
				expired := time.Duration(timestamp-time.Now().Unix()) * time.Second
				link.Expiration = &expired
			}
		}
	*/
	return link, nil
}

func (xc *XunLeiXCommon) MakeDir(ctx context.Context, parentDir model.Obj, dirName string) error {
	_, err := xc.Request(FILE_API_URL, http.MethodPost, func(r *resty.Request) {
		r.SetContext(ctx)
		r.SetBody(&base.Json{
			"kind":      FOLDER,
			"name":      dirName,
			"parent_id": parentDir.GetID(),
		})
	}, nil)
	return err
}

func (xc *XunLeiXCommon) Move(ctx context.Context, srcObj, dstDir model.Obj) error {
	_, err := xc.Request(FILE_API_URL+":batchMove", http.MethodPost, func(r *resty.Request) {
		r.SetContext(ctx)
		r.SetBody(&base.Json{
			"to":  base.Json{"parent_id": dstDir.GetID()},
			"ids": []string{srcObj.GetID()},
		})
	}, nil)
	return err
}

func (xc *XunLeiXCommon) Rename(ctx context.Context, srcObj model.Obj, newName string) error {
	_, err := xc.Request(FILE_API_URL+"/{fileID}", http.MethodPatch, func(r *resty.Request) {
		r.SetContext(ctx)
		r.SetPathParam("fileID", srcObj.GetID())
		r.SetBody(&base.Json{"name": newName})
	}, nil)
	return err
}

func (xc *XunLeiXCommon) Copy(ctx context.Context, srcObj, dstDir model.Obj) error {
	_, err := xc.Request(FILE_API_URL+":batchCopy", http.MethodPost, func(r *resty.Request) {
		r.SetContext(ctx)
		r.SetBody(&base.Json{
			"to":  base.Json{"parent_id": dstDir.GetID()},
			"ids": []string{srcObj.GetID()},
		})
	}, nil)
	return err
}

func (xc *XunLeiXCommon) Remove(ctx context.Context, obj model.Obj) error {
	_, err := xc.Request(FILE_API_URL+"/{fileID}/trash", http.MethodPatch, func(r *resty.Request) {
		r.SetContext(ctx)
		r.SetPathParam("fileID", obj.GetID())
		r.SetBody("{}")
	}, nil)
	return err
}

func (xc *XunLeiXCommon) Put(ctx context.Context, dstDir model.Obj, file model.FileStreamer, up driver.UpdateProgress) error {
	gcid := file.GetHash().GetHash(hash_extend.GCID)
	var err error
	if len(gcid) < hash_extend.GCID.Width {
		_, gcid, err = stream.CacheFullAndHash(file, &up, hash_extend.GCID, file.GetSize())
		if err != nil {
			return err
		}
	}

	var resp UploadTaskResponse
	_, err = xc.Request(FILE_API_URL, http.MethodPost, func(r *resty.Request) {
		r.SetContext(ctx)
		r.SetBody(&base.Json{
			"kind":        FILE,
			"parent_id":   dstDir.GetID(),
			"name":        file.GetName(),
			"size":        file.GetSize(),
			"hash":        gcid,
			"upload_type": UPLOAD_TYPE_RESUMABLE,
		})
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
		if file.GetSize() > s3manager.MaxUploadParts*s3manager.DefaultUploadPartSize {
			uploader.PartSize = file.GetSize() / (s3manager.MaxUploadParts - 1)
		}
		_, err = uploader.UploadWithContext(ctx, &s3manager.UploadInput{
			Bucket:  aws.String(param.Bucket),
			Key:     aws.String(param.Key),
			Expires: aws.Time(param.Expiration),
			Body: driver.NewLimitedUploadStream(ctx, &driver.ReaderUpdatingProgress{
				Reader:         file,
				UpdateProgress: up,
			}),
		})
		return err
	}
	return nil
}

func (xc *XunLeiXCommon) getFiles(ctx context.Context, folderId string) ([]model.Obj, error) {
	files := make([]model.Obj, 0)
	var pageToken string
	for {
		var fileList FileList
		_, err := xc.Request(FILE_API_URL, http.MethodGet, func(r *resty.Request) {
			r.SetContext(ctx)
			r.SetQueryParams(map[string]string{
				"space":      "",
				"__type":     "drive",
				"refresh":    "true",
				"__sync":     "true",
				"parent_id":  folderId,
				"page_token": pageToken,
				"with_audit": "true",
				"limit":      "100",
				"filters":    `{"phase":{"eq":"PHASE_TYPE_COMPLETE"},"trashed":{"eq":false}}`,
			})
		}, &fileList)
		if err != nil {
			return nil, err
		}

		for i := 0; i < len(fileList.Files); i++ {
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
func (xc *XunLeiXCommon) SetRefreshTokenFunc(fn func() error) {
	xc.refreshTokenFunc = fn
}

// SetTokenResp 设置Token
func (xc *XunLeiXCommon) SetTokenResp(tr *TokenResp) {
	xc.TokenResp = tr
}

// Request 携带Authorization和CaptchaToken的请�?
func (xc *XunLeiXCommon) Request(url string, method string, callback base.ReqCallback, resp interface{}) ([]byte, error) {
	data, err := xc.Common.Request(url, method, func(req *resty.Request) {
		req.SetHeaders(map[string]string{
			"Authorization":   xc.Token(),
			"X-Captcha-Token": xc.GetCaptchaToken(),
		})
		if callback != nil {
			callback(req)
		}
	}, resp)

	var errResp *ErrResp
	ok := errors.As(err, &errResp)
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
	case 9: // 验证码token过期
		if err = xc.RefreshCaptchaTokenAtLogin(GetAction(method, url), xc.UserID); err != nil {
			return nil, err
		}
	default:
		return nil, err
	}
	return xc.Request(url, method, callback, resp)
}

// RefreshToken 刷新Token
func (xc *XunLeiXCommon) RefreshToken(refreshToken string) (*TokenResp, error) {
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
		return nil, errs.EmptyToken
	}
	resp.UserID = resp.Sub
	return &resp, nil
}

// Login 登录
func (xc *XunLeiXCommon) Login(username, password string) (*TokenResp, error) {
	url := XLUSER_API_URL + "/auth/signin"
	err := xc.RefreshCaptchaTokenInLogin(GetAction(http.MethodPost, url), username)
	if err != nil {
		return nil, err
	}

	var resp TokenResp
	_, err = xc.Common.Request(url, http.MethodPost, func(req *resty.Request) {
		req.SetBody(&SignInRequest{
			CaptchaToken: xc.GetCaptchaToken(),
			ClientID:     xc.ClientID,
			ClientSecret: xc.ClientSecret,
			Username:     username,
			Password:     password,
		})
	}, &resp)
	if err != nil {
		return nil, err
	}
	resp.UserID = resp.Sub
	return &resp, nil
}

func (xc *XunLeiXCommon) IsLogin() bool {
	if xc.TokenResp == nil {
		return false
	}
	_, err := xc.Request(XLUSER_API_URL+"/user/me", http.MethodGet, nil, nil)
	return err == nil
}

// 离线下载文件，都和Pikpak接口一�?
func (xc *XunLeiXCommon) OfflineDownload(ctx context.Context, fileUrl string, parentDir model.Obj, fileName string) (*OfflineTask, error) {
	requestBody := base.Json{
		"kind":        "drive#file",
		"name":        fileName,
		"upload_type": "UPLOAD_TYPE_URL",
		"url": base.Json{
			"url": fileUrl,
		},
		"params":    base.Json{},
		"parent_id": parentDir.GetID(),
	}
	var resp OfflineDownloadResp // 一样的
	_, err := xc.Request(FILE_API_URL, http.MethodPost, func(req *resty.Request) {
		req.SetContext(ctx).
			SetBody(requestBody)
	}, &resp)

	if err != nil {
		return nil, err
	}

	return &resp.Task, err
}

// 获取离线下载任务列表
func (xc *XunLeiXCommon) OfflineList(ctx context.Context, nextPageToken string, phase []string) ([]OfflineTask, error) {
	res := make([]OfflineTask, 0)
	if len(phase) == 0 {
		phase = []string{"PHASE_TYPE_RUNNING", "PHASE_TYPE_ERROR", "PHASE_TYPE_COMPLETE", "PHASE_TYPE_PENDING"}
	}
	params := map[string]string{
		"type":           "offline",
		"thumbnail_size": "SIZE_SMALL",
		"limit":          "10000",
		"page_token":     nextPageToken,
		"with":           "reference_resource",
	}

	// 处理 phase 参数
	if len(phase) > 0 {
		filters := base.Json{
			"phase": map[string]string{
				"in": strings.Join(phase, ","),
			},
		}
		filtersJSON, err := json.Marshal(filters)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal filters: %w", err)
		}
		params["filters"] = string(filtersJSON)
	}

	var resp OfflineListResp
	_, err := xc.Request(TASKS_API_URL, http.MethodGet, func(req *resty.Request) {
		req.SetContext(ctx).
			SetQueryParams(params)
	}, &resp)

	if err != nil {
		return nil, fmt.Errorf("failed to get offline list: %w", err)
	}
	res = append(res, resp.Tasks...)
	return res, nil
}

func (xc *XunLeiXCommon) DeleteOfflineTasks(ctx context.Context, taskIDs []string, deleteFiles bool) error {
	params := map[string]string{
		"task_ids":     strings.Join(taskIDs, ","),
		"delete_files": strconv.FormatBool(deleteFiles),
	}
	_, err := xc.Request(TASKS_API_URL, http.MethodDelete, func(req *resty.Request) {
		req.SetContext(ctx).
			SetQueryParams(params)
	}, nil)
	if err != nil {
		return fmt.Errorf("failed to delete tasks %v: %w", taskIDs, err)
	}
	return nil
}
