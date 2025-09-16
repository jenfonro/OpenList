package doubao

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"math"
	"math/rand"
	"net/http"
	"net/url"
	stdpath "path"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/OpenListTeam/OpenList/v4/drivers/base"
	"github.com/OpenListTeam/OpenList/v4/internal/driver"
	"github.com/OpenListTeam/OpenList/v4/internal/model"
	"github.com/OpenListTeam/OpenList/v4/internal/stream"
	"github.com/OpenListTeam/OpenList/v4/pkg/errgroup"
	"github.com/OpenListTeam/OpenList/v4/pkg/utils"
	"github.com/avast/retry-go"
	"github.com/go-resty/resty/v2"
	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"
)

const (
	DirectoryType      = 1
	FileType           = 2
	LinkType           = 3
	ImageType          = 4
	PagesType          = 5
	VideoType          = 6
	AudioType          = 7
	MeetingMinutesType = 8
)

var FileNodeType = map[int]string{
	1: "directory",
	2: "file",
	3: "link",
	4: "image",
	5: "pages",
	6: "video",
	7: "audio",
	8: "meeting_minutes",
}

const (
	BaseURL          = "https://www.doubao.com"
	FileDataType     = "file"
	ImgDataType      = "image"
	VideoDataType    = "video"
	DefaultChunkSize = int64(5 * 1024 * 1024) // 5MB
	MaxRetryAttempts = 3                      // 最大重试次�?
	UserAgent        = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/129.0.0.0 Safari/537.36"
	Region           = "cn-north-1"
	UploadTimeout    = 3 * time.Minute
)

// do others that not defined in Driver interface
func (d *Doubao) request(path string, method string, callback base.ReqCallback, resp interface{}) ([]byte, error) {
	reqUrl := BaseURL + path
	req := base.RWithProxy(d.DriverProxyAddr)
	req.SetHeader("Cookie", d.Cookie)
	if callback != nil {
		callback(req)
	}

	var commonResp CommonResp

	res, err := req.Execute(method, reqUrl)
	log.Debugln(res.String())
	if err != nil {
		return nil, err
	}

	body := res.Body()
	// 先解析为通用响应
	if err = json.Unmarshal(body, &commonResp); err != nil {
		return nil, err
	}
	// 检查响应是否成�?
	if !commonResp.IsSuccess() {
		return body, commonResp.GetError()
	}

	if resp != nil {
		if err = json.Unmarshal(body, resp); err != nil {
			return body, err
		}
	}

	return body, nil
}

func (d *Doubao) getFiles(dirId, cursor string) (resp []File, err error) {
	var r NodeInfoResp

	var body = base.Json{
		"node_id": dirId,
	}
	// 如果有游标，则设置游标和大小
	if cursor != "" {
		body["cursor"] = cursor
		body["size"] = 50
	} else {
		body["need_full_path"] = false
	}

	_, err = d.request("/samantha/aispace/node_info", http.MethodPost, func(req *resty.Request) {
		req.SetBody(body)
	}, &r)
	if err != nil {
		return nil, err
	}

	if r.Data.Children != nil {
		resp = r.Data.Children
	}

	if r.Data.NextCursor != "-1" {
		// 递归获取下一�?
		nextFiles, err := d.getFiles(dirId, r.Data.NextCursor)
		if err != nil {
			return nil, err
		}

		resp = append(r.Data.Children, nextFiles...)
	}

	return resp, err
}

func (d *Doubao) getUserInfo() (UserInfo, error) {
	var r UserInfoResp

	_, err := d.request("/passport/account/info/v2/", http.MethodGet, nil, &r)
	if err != nil {
		return UserInfo{}, err
	}

	return r.Data, err
}

// 签名请求
func (d *Doubao) signRequest(req *resty.Request, method, tokenType, uploadUrl string) error {
	parsedUrl, err := url.Parse(uploadUrl)
	if err != nil {
		return fmt.Errorf("invalid URL format: %w", err)
	}

	var accessKeyId, secretAccessKey, sessionToken string
	var serviceName string

	if tokenType == VideoDataType {
		accessKeyId = d.UploadToken.Samantha.StsToken.AccessKeyID
		secretAccessKey = d.UploadToken.Samantha.StsToken.SecretAccessKey
		sessionToken = d.UploadToken.Samantha.StsToken.SessionToken
		serviceName = "vod"
	} else {
		accessKeyId = d.UploadToken.Alice[tokenType].Auth.AccessKeyID
		secretAccessKey = d.UploadToken.Alice[tokenType].Auth.SecretAccessKey
		sessionToken = d.UploadToken.Alice[tokenType].Auth.SessionToken
		serviceName = "imagex"
	}

	// 当前时间，格式为 ISO8601
	now := time.Now().UTC()
	amzDate := now.Format("20060102T150405Z")
	dateStamp := now.Format("20060102")

	req.SetHeader("X-Amz-Date", amzDate)

	if sessionToken != "" {
		req.SetHeader("X-Amz-Security-Token", sessionToken)
	}

	// 计算请求体的SHA256哈希
	var bodyHash string
	if req.Body != nil {
		bodyBytes, ok := req.Body.([]byte)
		if !ok {
			return fmt.Errorf("request body must be []byte")
		}

		bodyHash = hashSHA256(string(bodyBytes))
		req.SetHeader("X-Amz-Content-Sha256", bodyHash)
	} else {
		bodyHash = hashSHA256("")
	}

	// 创建规范请求
	canonicalURI := parsedUrl.Path
	if canonicalURI == "" {
		canonicalURI = "/"
	}

	// 查询参数按照字母顺序排序
	canonicalQueryString := getCanonicalQueryString(req.QueryParam)
	// 规范请求�?
	canonicalHeaders, signedHeaders := getCanonicalHeadersFromMap(req.Header)
	canonicalRequest := method + "\n" +
		canonicalURI + "\n" +
		canonicalQueryString + "\n" +
		canonicalHeaders + "\n" +
		signedHeaders + "\n" +
		bodyHash

	algorithm := "AWS4-HMAC-SHA256"
	credentialScope := fmt.Sprintf("%s/%s/%s/aws4_request", dateStamp, Region, serviceName)

	stringToSign := algorithm + "\n" +
		amzDate + "\n" +
		credentialScope + "\n" +
		hashSHA256(canonicalRequest)
	// 计算签名密钥
	signingKey := getSigningKey(secretAccessKey, dateStamp, Region, serviceName)
	// 计算签名
	signature := hmacSHA256Hex(signingKey, stringToSign)
	// 构建授权�?
	authorizationHeader := fmt.Sprintf(
		"%s Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		algorithm,
		accessKeyId,
		credentialScope,
		signedHeaders,
		signature,
	)

	req.SetHeader("Authorization", authorizationHeader)

	return nil
}

func (d *Doubao) requestApi(url, method, tokenType string, callback base.ReqCallback, resp interface{}) ([]byte, error) {
	req := base.RWithProxy(d.DriverProxyAddr)
	req.SetHeaders(map[string]string{
		"user-agent": UserAgent,
	})

	if method == http.MethodPost {
		req.SetHeader("Content-Type", "text/plain;charset=UTF-8")
	}

	if callback != nil {
		callback(req)
	}

	if resp != nil {
		req.SetResult(resp)
	}

	// 使用自定义AWS SigV4签名
	err := d.signRequest(req, method, tokenType, url)
	if err != nil {
		return nil, err
	}

	res, err := req.Execute(method, url)
	if err != nil {
		return nil, err
	}

	return res.Body(), nil
}

func (d *Doubao) initUploadToken() (*UploadToken, error) {
	uploadToken := &UploadToken{
		Alice:    make(map[string]UploadAuthToken),
		Samantha: MediaUploadAuthToken{},
	}

	fileAuthToken, err := d.getUploadAuthToken(FileDataType)
	if err != nil {
		return nil, err
	}

	imgAuthToken, err := d.getUploadAuthToken(ImgDataType)
	if err != nil {
		return nil, err
	}

	mediaAuthToken, err := d.getSamantaUploadAuthToken()
	if err != nil {
		return nil, err
	}

	uploadToken.Alice[FileDataType] = fileAuthToken
	uploadToken.Alice[ImgDataType] = imgAuthToken
	uploadToken.Samantha = mediaAuthToken

	return uploadToken, nil
}

func (d *Doubao) getUploadAuthToken(dataType string) (ut UploadAuthToken, err error) {
	var r UploadAuthTokenResp
	_, err = d.request("/alice/upload/auth_token", http.MethodPost, func(req *resty.Request) {
		req.SetBody(base.Json{
			"scene":     "bot_chat",
			"data_type": dataType,
		})
	}, &r)

	return r.Data, err
}

func (d *Doubao) getSamantaUploadAuthToken() (mt MediaUploadAuthToken, err error) {
	var r MediaUploadAuthTokenResp
	_, err = d.request("/samantha/media/get_upload_token", http.MethodPost, func(req *resty.Request) {
		req.SetBody(base.Json{})
	}, &r)

	return r.Data, err
}

// getUploadConfig 获取上传配置信息
func (d *Doubao) getUploadConfig(upConfig *UploadConfig, dataType string, file model.FileStreamer) error {
	tokenType := dataType
	// 配置参数函数
	configureParams := func() (string, map[string]string) {
		var uploadUrl string
		var params map[string]string
		// 根据数据类型设置不同的上传参�?
		switch dataType {
		case VideoDataType:
			// 音频/视频类型 - 使用uploadToken.Samantha的配�?
			uploadUrl = d.UploadToken.Samantha.UploadInfo.VideoHost
			params = map[string]string{
				"Action":       "ApplyUploadInner",
				"Version":      "2020-11-19",
				"SpaceName":    d.UploadToken.Samantha.UploadInfo.SpaceName,
				"FileType":     "video",
				"IsInner":      "1",
				"NeedFallback": "true",
				"FileSize":     strconv.FormatInt(file.GetSize(), 10),
				"s":            randomString(),
			}
		case ImgDataType, FileDataType:
			// 图片或其他文件类�?- 使用uploadToken.Alice对应配置
			uploadUrl = "https://" + d.UploadToken.Alice[dataType].UploadHost
			params = map[string]string{
				"Action":        "ApplyImageUpload",
				"Version":       "2018-08-01",
				"ServiceId":     d.UploadToken.Alice[dataType].ServiceID,
				"NeedFallback":  "true",
				"FileSize":      strconv.FormatInt(file.GetSize(), 10),
				"FileExtension": stdpath.Ext(file.GetName()),
				"s":             randomString(),
			}
		}
		return uploadUrl, params
	}

	// 获取初始参数
	uploadUrl, params := configureParams()

	tokenRefreshed := false
	var configResp UploadConfigResp

	err := d._retryOperation("get upload_config", func() error {
		configResp = UploadConfigResp{}

		_, err := d.requestApi(uploadUrl, http.MethodGet, tokenType, func(req *resty.Request) {
			req.SetQueryParams(params)
		}, &configResp)
		if err != nil {
			return err
		}

		if configResp.ResponseMetadata.Error.Code == "" {
			*upConfig = configResp.Result
			return nil
		}

		// 100028 凭证过期
		if configResp.ResponseMetadata.Error.CodeN == 100028 && !tokenRefreshed {
			log.Debugln("[doubao] Upload token expired, re-fetching...")
			newToken, err := d.initUploadToken()
			if err != nil {
				return fmt.Errorf("failed to refresh token: %w", err)
			}

			d.UploadToken = newToken
			tokenRefreshed = true
			uploadUrl, params = configureParams()

			return retry.Error{errors.New("token refreshed, retry needed")}
		}

		return fmt.Errorf("get upload_config failed: %s", configResp.ResponseMetadata.Error.Message)
	})

	return err
}

// uploadNode 上传 文件信息
func (d *Doubao) uploadNode(uploadConfig *UploadConfig, dir model.Obj, file model.FileStreamer, dataType string) (UploadNodeResp, error) {
	reqUuid := uuid.New().String()
	var key string
	var nodeType int

	mimetype := file.GetMimetype()
	switch dataType {
	case VideoDataType:
		key = uploadConfig.InnerUploadAddress.UploadNodes[0].Vid
		if strings.HasPrefix(mimetype, "audio/") {
			nodeType = AudioType // 音频类型
		} else {
			nodeType = VideoType // 视频类型
		}
	case ImgDataType:
		key = uploadConfig.InnerUploadAddress.UploadNodes[0].StoreInfos[0].StoreURI
		nodeType = ImageType // 图片类型
	default: // FileDataType
		key = uploadConfig.InnerUploadAddress.UploadNodes[0].StoreInfos[0].StoreURI
		nodeType = FileType // 文件类型
	}

	var r UploadNodeResp
	_, err := d.request("/samantha/aispace/upload_node", http.MethodPost, func(req *resty.Request) {
		req.SetBody(base.Json{
			"node_list": []base.Json{
				{
					"local_id":     reqUuid,
					"parent_id":    dir.GetID(),
					"name":         file.GetName(),
					"key":          key,
					"node_content": base.Json{},
					"node_type":    nodeType,
					"size":         file.GetSize(),
				},
			},
			"request_id": reqUuid,
		})
	}, &r)

	return r, err
}

// Upload 普通上传实�?
func (d *Doubao) Upload(ctx context.Context, config *UploadConfig, dstDir model.Obj, file model.FileStreamer, up driver.UpdateProgress, dataType string) (model.Obj, error) {
	ss, err := stream.NewStreamSectionReader(file, int(file.GetSize()), &up)
	if err != nil {
		return nil, err
	}

	reader, err := ss.GetSectionReader(0, file.GetSize())
	if err != nil {
		return nil, err
	}

	// 计算CRC32
	crc32Hash := crc32.NewIEEE()
	w, err := utils.CopyWithBuffer(crc32Hash, reader)
	if w != file.GetSize() {
		return nil, fmt.Errorf("failed to read all data: (expect =%d, actual =%d) %w", file.GetSize(), w, err)
	}
	crc32Value := hex.EncodeToString(crc32Hash.Sum(nil))

	// 构建请求路径
	uploadNode := config.InnerUploadAddress.UploadNodes[0]
	storeInfo := uploadNode.StoreInfos[0]
	uploadUrl := fmt.Sprintf("https://%s/upload/v1/%s", uploadNode.UploadHost, storeInfo.StoreURI)
	rateLimitedRd := driver.NewLimitedUploadStream(ctx, reader)
	err = d._retryOperation("Upload", func() error {
		reader.Seek(0, io.SeekStart)
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, uploadUrl, rateLimitedRd)
		if err != nil {
			return err
		}
		req.Header = map[string][]string{
			"Referer":             {BaseURL + "/"},
			"Origin":              {BaseURL},
			"User-Agent":          {UserAgent},
			"X-Storage-U":         {d.UserId},
			"Authorization":       {storeInfo.Auth},
			"Content-Type":        {"application/octet-stream"},
			"Content-Crc32":       {crc32Value},
			"Content-Length":      {fmt.Sprintf("%d", file.GetSize())},
			"Content-Disposition": {fmt.Sprintf("attachment; filename=%s", url.QueryEscape(storeInfo.StoreURI))},
		}
		res, err := base.HttpClient.Do(req)
		if err != nil {
			return err
		}
		defer res.Body.Close()
		bytes, _ := io.ReadAll(res.Body)
		resp := UploadResp{}
		utils.Json.Unmarshal(bytes, &resp)
		if resp.Code != 2000 {
			return fmt.Errorf("upload part failed: %s", resp.Message)
		} else if resp.Data.Crc32 != crc32Value {
			return fmt.Errorf("upload part failed: crc32 mismatch, expected %s, got %s", crc32Value, resp.Data.Crc32)
		}
		return nil
	})
	ss.FreeSectionReader(reader)
	if err != nil {
		return nil, err
	}

	uploadNodeResp, err := d.uploadNode(config, dstDir, file, dataType)
	if err != nil {
		return nil, err
	}

	return &model.Object{
		ID:       uploadNodeResp.Data.NodeList[0].ID,
		Name:     uploadNodeResp.Data.NodeList[0].Name,
		Size:     file.GetSize(),
		IsFolder: false,
	}, nil
}

// UploadByMultipart 分片上传
func (d *Doubao) UploadByMultipart(ctx context.Context, config *UploadConfig, fileSize int64, dstDir model.Obj, file model.FileStreamer, up driver.UpdateProgress, dataType string) (model.Obj, error) {
	// 构建请求路径
	uploadNode := config.InnerUploadAddress.UploadNodes[0]
	storeInfo := uploadNode.StoreInfos[0]
	uploadUrl := fmt.Sprintf("https://%s/upload/v1/%s", uploadNode.UploadHost, storeInfo.StoreURI)
	// 初始化分片上�?
	var uploadID string
	err := d._retryOperation("Initialize multipart upload", func() error {
		var err error
		uploadID, err = d.initMultipartUpload(config, uploadUrl, storeInfo)
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("failed to initialize multipart upload: %w", err)
	}
	// 准备分片参数
	chunkSize := DefaultChunkSize
	if config.InnerUploadAddress.AdvanceOption.SliceSize > 0 {
		chunkSize = int64(config.InnerUploadAddress.AdvanceOption.SliceSize)
	}
	ss, err := stream.NewStreamSectionReader(file, int(chunkSize), &up)
	if err != nil {
		return nil, err
	}

	totalParts := (fileSize + chunkSize - 1) / chunkSize
	// 创建分片信息�?
	parts := make([]UploadPart, totalParts)

	up(10.0) // 更新进度
	// 设置并行上传
	thread := min(int(totalParts), d.uploadThread)
	threadG, uploadCtx := errgroup.NewOrderedGroupWithContext(ctx, thread,
		retry.Attempts(MaxRetryAttempts),
		retry.Delay(time.Second),
		retry.DelayType(retry.BackOffDelay),
		retry.MaxJitter(200*time.Millisecond),
	)

	var partsMutex sync.Mutex
	// 并行上传所有分�?
	hash := crc32.NewIEEE()
	for partIndex := range totalParts {
		if utils.IsCanceled(uploadCtx) {
			break
		}
		partNumber := partIndex + 1 // 分片编号�?开�?

		// 计算此分片的大小和偏�?
		offset := partIndex * chunkSize
		size := chunkSize
		if partIndex == totalParts-1 {
			size = fileSize - offset
		}
		var reader *stream.SectionReader
		var rateLimitedRd io.Reader
		crc32Value := ""
		threadG.GoWithLifecycle(errgroup.Lifecycle{
			Before: func(ctx context.Context) error {
				if reader == nil {
					var err error
					reader, err = ss.GetSectionReader(offset, size)
					if err != nil {
						return err
					}
					hash.Reset()
					w, err := utils.CopyWithBuffer(hash, reader)
					if w != size {
						return fmt.Errorf("failed to read all data: (expect =%d, actual =%d) %w", size, w, err)
					}
					crc32Value = hex.EncodeToString(hash.Sum(nil))
					rateLimitedRd = driver.NewLimitedUploadStream(ctx, reader)
				}
				return nil
			},
			Do: func(ctx context.Context) error {
				reader.Seek(0, io.SeekStart)
				req, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("%s?uploadid=%s&part_number=%d&phase=transfer", uploadUrl, uploadID, partNumber), rateLimitedRd)
				if err != nil {
					return err
				}
				req.Header = map[string][]string{
					"Referer":             {BaseURL + "/"},
					"Origin":              {BaseURL},
					"User-Agent":          {UserAgent},
					"X-Storage-U":         {d.UserId},
					"Authorization":       {storeInfo.Auth},
					"Content-Type":        {"application/octet-stream"},
					"Content-Crc32":       {crc32Value},
					"Content-Length":      {fmt.Sprintf("%d", size)},
					"Content-Disposition": {fmt.Sprintf("attachment; filename=%s", url.QueryEscape(storeInfo.StoreURI))},
				}
				res, err := base.HttpClient.Do(req)
				if err != nil {
					return err
				}
				defer res.Body.Close()
				bytes, _ := io.ReadAll(res.Body)
				uploadResp := UploadResp{}
				utils.Json.Unmarshal(bytes, &uploadResp)
				if uploadResp.Code != 2000 {
					return fmt.Errorf("upload part failed: %s", uploadResp.Message)
				} else if uploadResp.Data.Crc32 != crc32Value {
					return fmt.Errorf("upload part failed: crc32 mismatch, expected %s, got %s", crc32Value, uploadResp.Data.Crc32)
				}
				// 记录成功上传的分�?
				partsMutex.Lock()
				parts[partIndex] = UploadPart{
					PartNumber: strconv.FormatInt(partNumber, 10),
					Etag:       uploadResp.Data.Etag,
					Crc32:      crc32Value,
				}
				partsMutex.Unlock()
				// 更新进度
				progress := 10.0 + 90.0*float64(threadG.Success()+1)/float64(totalParts)
				up(math.Min(progress, 95.0))
				return nil
			},
			After: func(err error) {
				ss.FreeSectionReader(reader)
			},
		})
	}

	if err = threadG.Wait(); err != nil {
		return nil, err
	}
	// 完成上传-分片合并
	if err = d._retryOperation("Complete multipart upload", func() error {
		return d.completeMultipartUpload(config, uploadUrl, uploadID, parts)
	}); err != nil {
		return nil, fmt.Errorf("failed to complete multipart upload: %w", err)
	}
	// 提交上传
	if err = d._retryOperation("Commit upload", func() error {
		return d.commitMultipartUpload(config)
	}); err != nil {
		return nil, fmt.Errorf("failed to commit upload: %w", err)
	}

	up(98.0) // 更新�?8%
	// 上传节点信息
	var uploadNodeResp UploadNodeResp

	if err = d._retryOperation("Upload node", func() error {
		var err error
		uploadNodeResp, err = d.uploadNode(config, dstDir, file, dataType)
		return err
	}); err != nil {
		return nil, fmt.Errorf("failed to upload node: %w", err)
	}

	up(100.0) // 完成上传

	return &model.Object{
		ID:       uploadNodeResp.Data.NodeList[0].ID,
		Name:     uploadNodeResp.Data.NodeList[0].Name,
		Size:     file.GetSize(),
		IsFolder: false,
	}, nil
}

// 统一上传请求方法
func (d *Doubao) uploadRequest(uploadUrl string, method string, storeInfo StoreInfo, callback base.ReqCallback, resp interface{}) ([]byte, error) {
	client := resty.New()
	client.SetTransport(&http.Transport{
		DisableKeepAlives: true,  // 禁用连接复用
		ForceAttemptHTTP2: false, // 强制使用HTTP/1.1
	})
	client.SetTimeout(UploadTimeout)

	req := client.R()
	req.SetHeaders(map[string]string{
		"Host":          strings.Split(uploadUrl, "/")[2],
		"Referer":       BaseURL + "/",
		"Origin":        BaseURL,
		"User-Agent":    UserAgent,
		"X-Storage-U":   d.UserId,
		"Authorization": storeInfo.Auth,
	})

	if method == http.MethodPost {
		req.SetHeader("Content-Type", "text/plain;charset=UTF-8")
	}

	if callback != nil {
		callback(req)
	}

	if resp != nil {
		req.SetResult(resp)
	}

	res, err := req.Execute(method, uploadUrl)
	if err != nil && err != io.EOF {
		return nil, fmt.Errorf("upload request failed: %w", err)
	}

	return res.Body(), nil
}

// 初始化分片上�?
func (d *Doubao) initMultipartUpload(config *UploadConfig, uploadUrl string, storeInfo StoreInfo) (uploadId string, err error) {
	uploadResp := UploadResp{}

	_, err = d.uploadRequest(uploadUrl, http.MethodPost, storeInfo, func(req *resty.Request) {
		req.SetQueryParams(map[string]string{
			"uploadmode": "part",
			"phase":      "init",
		})
	}, &uploadResp)

	if err != nil {
		return uploadId, err
	}

	if uploadResp.Code != 2000 {
		return uploadId, fmt.Errorf("init upload failed: %s", uploadResp.Message)
	}

	return uploadResp.Data.UploadId, nil
}

// 完成分片上传
func (d *Doubao) completeMultipartUpload(config *UploadConfig, uploadUrl, uploadID string, parts []UploadPart) error {
	uploadResp := UploadResp{}

	storeInfo := config.InnerUploadAddress.UploadNodes[0].StoreInfos[0]

	body := _convertUploadParts(parts)

	err := utils.Retry(MaxRetryAttempts, time.Second, func() (err error) {
		_, err = d.uploadRequest(uploadUrl, http.MethodPost, storeInfo, func(req *resty.Request) {
			req.SetQueryParams(map[string]string{
				"uploadid":   uploadID,
				"phase":      "finish",
				"uploadmode": "part",
			})
			req.SetBody(body)
		}, &uploadResp)

		if err != nil {
			return err
		}
		// 检查响应状态码 2000 成功 4024 分片合并�?
		if uploadResp.Code != 2000 && uploadResp.Code != 4024 {
			return fmt.Errorf("finish upload failed: %s", uploadResp.Message)
		}

		return err
	})

	if err != nil {
		return fmt.Errorf("failed to complete multipart upload: %w", err)
	}

	return nil
}

func (d *Doubao) commitMultipartUpload(uploadConfig *UploadConfig) error {
	uploadUrl := d.UploadToken.Samantha.UploadInfo.VideoHost
	params := map[string]string{
		"Action":    "CommitUploadInner",
		"Version":   "2020-11-19",
		"SpaceName": d.UploadToken.Samantha.UploadInfo.SpaceName,
	}
	tokenType := VideoDataType

	videoCommitUploadResp := VideoCommitUploadResp{}

	jsonBytes, err := json.Marshal(base.Json{
		"SessionKey": uploadConfig.InnerUploadAddress.UploadNodes[0].SessionKey,
		"Functions":  []base.Json{},
	})
	if err != nil {
		return fmt.Errorf("failed to marshal request data: %w", err)
	}

	_, err = d.requestApi(uploadUrl, http.MethodPost, tokenType, func(req *resty.Request) {
		req.SetHeader("Content-Type", "application/json")
		req.SetQueryParams(params)
		req.SetBody(jsonBytes)

	}, &videoCommitUploadResp)
	if err != nil {
		return err
	}

	return nil
}

// _retryOperation 操作重试
func (d *Doubao) _retryOperation(operation string, fn func() error) error {
	return retry.Do(
		fn,
		retry.Attempts(MaxRetryAttempts),
		retry.Delay(500*time.Millisecond),
		retry.DelayType(retry.BackOffDelay),
		retry.MaxJitter(200*time.Millisecond),
		retry.OnRetry(func(n uint, err error) {
			log.Debugf("[doubao] %s retry #%d: %v", operation, n+1, err)
		}),
	)
}

// _convertUploadParts 将分片信息转换为字符�?
func _convertUploadParts(parts []UploadPart) string {
	if len(parts) == 0 {
		return ""
	}

	var result strings.Builder

	for i, part := range parts {
		if i > 0 {
			result.WriteString(",")
		}
		result.WriteString(fmt.Sprintf("%s:%s", part.PartNumber, part.Crc32))
	}

	return result.String()
}

// 获取规范查询字符�?
func getCanonicalQueryString(query url.Values) string {
	if len(query) == 0 {
		return ""
	}

	keys := make([]string, 0, len(query))
	for k := range query {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		values := query[k]
		for _, v := range values {
			parts = append(parts, urlEncode(k)+"="+urlEncode(v))
		}
	}

	return strings.Join(parts, "&")
}

func urlEncode(s string) string {
	s = url.QueryEscape(s)
	s = strings.ReplaceAll(s, "+", "%20")
	return s
}

// 获取规范头信息和已签名头列表
func getCanonicalHeadersFromMap(headers map[string][]string) (string, string) {
	// 不可签名的头部列�?
	unsignableHeaders := map[string]bool{
		"authorization":     true,
		"content-type":      true,
		"content-length":    true,
		"user-agent":        true,
		"presigned-expires": true,
		"expect":            true,
		"x-amzn-trace-id":   true,
	}
	headerValues := make(map[string]string)
	var signedHeadersList []string

	for k, v := range headers {
		if len(v) == 0 {
			continue
		}

		lowerKey := strings.ToLower(k)
		// 检查是否可签名
		if strings.HasPrefix(lowerKey, "x-amz-") || !unsignableHeaders[lowerKey] {
			value := strings.TrimSpace(v[0])
			value = strings.Join(strings.Fields(value), " ")
			headerValues[lowerKey] = value
			signedHeadersList = append(signedHeadersList, lowerKey)
		}
	}

	sort.Strings(signedHeadersList)

	var canonicalHeadersStr strings.Builder
	for _, key := range signedHeadersList {
		canonicalHeadersStr.WriteString(key)
		canonicalHeadersStr.WriteString(":")
		canonicalHeadersStr.WriteString(headerValues[key])
		canonicalHeadersStr.WriteString("\n")
	}

	signedHeaders := strings.Join(signedHeadersList, ";")

	return canonicalHeadersStr.String(), signedHeaders
}

// 计算HMAC-SHA256
func hmacSHA256(key []byte, data string) []byte {
	h := hmac.New(sha256.New, key)
	h.Write([]byte(data))
	return h.Sum(nil)
}

// 计算HMAC-SHA256并返回十六进制字符串
func hmacSHA256Hex(key []byte, data string) string {
	return hex.EncodeToString(hmacSHA256(key, data))
}

// 计算SHA256哈希并返回十六进制字符串
func hashSHA256(data string) string {
	h := sha256.New()
	h.Write([]byte(data))
	return hex.EncodeToString(h.Sum(nil))
}

// 获取签名密钥
func getSigningKey(secretKey, dateStamp, region, service string) []byte {
	kDate := hmacSHA256([]byte("AWS4"+secretKey), dateStamp)
	kRegion := hmacSHA256(kDate, region)
	kService := hmacSHA256(kRegion, service)
	kSigning := hmacSHA256(kService, "aws4_request")
	return kSigning
}

func randomString() string {
	const charset = "0123456789abcdefghijklmnopqrstuvwxyz"
	const length = 11 // 11位随机字符串

	var sb strings.Builder
	sb.Grow(length)

	for i := 0; i < length; i++ {
		sb.WriteByte(charset[rand.Intn(len(charset))])
	}

	return sb.String()
}
