package doubao_share

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/OpenListTeam/OpenList/v4/drivers/base"
	"github.com/OpenListTeam/OpenList/v4/internal/model"
	"github.com/go-resty/resty/v2"
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
	BaseURL       = "https://www.doubao.com"
	FileDataType  = "file"
	ImgDataType   = "image"
	VideoDataType = "video"
	UserAgent     = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/129.0.0.0 Safari/537.36"
)

func (d *DoubaoShare) request(path string, method string, callback base.ReqCallback, resp interface{}) ([]byte, error) {
	reqUrl := BaseURL + path
	req := base.RWithProxy(d.DriverProxyAddr)

	req.SetHeaders(map[string]string{
		"Cookie":     d.Cookie,
		"User-Agent": UserAgent,
	})

	req.SetQueryParams(map[string]string{
		"version_code":    "20800",
		"device_platform": "web",
	})

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

func (d *DoubaoShare) getFiles(dirId, nodeId, cursor string) (resp []File, err error) {
	var r NodeInfoResp

	var body = base.Json{
		"share_id": dirId,
		"node_id":  nodeId,
	}
	// 如果有游标，则设置游标和大小
	if cursor != "" {
		body["cursor"] = cursor
		body["size"] = 50
	} else {
		body["need_full_path"] = false
	}

	_, err = d.request("/samantha/aispace/share/node_info", http.MethodPost, func(req *resty.Request) {
		req.SetBody(body)
	}, &r)
	if err != nil {
		return nil, err
	}

	if r.NodeInfoData.Children != nil {
		resp = r.NodeInfoData.Children
	}

	if r.NodeInfoData.NextCursor != "-1" {
		// 递归获取下一�?
		nextFiles, err := d.getFiles(dirId, nodeId, r.NodeInfoData.NextCursor)
		if err != nil {
			return nil, err
		}

		resp = append(r.NodeInfoData.Children, nextFiles...)
	}

	return resp, err
}

func (d *DoubaoShare) getShareOverview(shareId, cursor string) (resp []File, err error) {
	return d.getShareOverviewWithHistory(shareId, cursor, make(map[string]bool))
}

func (d *DoubaoShare) getShareOverviewWithHistory(shareId, cursor string, cursorHistory map[string]bool) (resp []File, err error) {
	var r NodeInfoResp

	var body = base.Json{
		"share_id": shareId,
	}
	// 如果有游标，则设置游标和大小
	if cursor != "" {
		body["cursor"] = cursor
		body["size"] = 50
	} else {
		body["need_full_path"] = false
	}

	_, err = d.request("/samantha/aispace/share/overview", http.MethodPost, func(req *resty.Request) {
		req.SetBody(body)
	}, &r)
	if err != nil {
		return nil, err
	}

	if r.NodeInfoData.NodeList != nil {
		resp = r.NodeInfoData.NodeList
	}

	if r.NodeInfoData.NextCursor != "-1" {
		// 检查游标是否重复出现，防止无限循环
		if cursorHistory[r.NodeInfoData.NextCursor] {
			return resp, nil
		}

		// 记录当前游标
		cursorHistory[r.NodeInfoData.NextCursor] = true

		// 递归获取下一�?
		nextFiles, err := d.getShareOverviewWithHistory(shareId, r.NodeInfoData.NextCursor, cursorHistory)
		if err != nil {
			return nil, err
		}

		resp = append(resp, nextFiles...)
	}

	return resp, nil
}

func (d *DoubaoShare) initShareList() error {
	if d.Addition.ShareIds == "" {
		return fmt.Errorf("share_ids is empty")
	}

	// 解析分享配置
	shareConfigs, rootShares, err := d._parseShareConfigs()
	if err != nil {
		return err
	}

	// 检查路径冲�?
	if err := d._detectPathConflicts(shareConfigs); err != nil {
		return err
	}

	// 构建树形结构
	rootMap := d._buildTreeStructure(shareConfigs, rootShares)

	// 提取顶级节点
	topLevelNodes := d._extractTopLevelNodes(rootMap, rootShares)
	if len(topLevelNodes) == 0 {
		return fmt.Errorf("no valid share_ids found")
	}

	// 存储结果
	d.RootFiles = topLevelNodes

	return nil
}

// 从配置中解析分享ID和路�?
func (d *DoubaoShare) _parseShareConfigs() (map[string]string, []string, error) {
	shareConfigs := make(map[string]string) // 路径 -> 分享ID
	rootShares := make([]string, 0)         // 根目录显示的分享ID

	lines := strings.Split(strings.TrimSpace(d.Addition.ShareIds), "\n")
	if len(lines) == 0 {
		return nil, nil, fmt.Errorf("no share_ids found")
	}

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// 解析分享ID和路�?
		parts := strings.Split(line, "|")
		var shareId, sharePath string

		if len(parts) == 1 {
			// 无路径分享，直接在根目录显示
			shareId = _extractShareId(parts[0])
			if shareId != "" {
				rootShares = append(rootShares, shareId)
			}
			continue
		} else if len(parts) >= 2 {
			shareId = _extractShareId(parts[0])
			sharePath = strings.Trim(parts[1], "/")
		}

		if shareId == "" {
			log.Warnf("[doubao_share] Invalid Share_id Format: %s", line)
			continue
		}

		// 空路径也加入根目录显�?
		if sharePath == "" {
			rootShares = append(rootShares, shareId)
			continue
		}

		// 添加到路径映�?
		shareConfigs[sharePath] = shareId
	}

	return shareConfigs, rootShares, nil
}

// 检测路径冲�?
func (d *DoubaoShare) _detectPathConflicts(shareConfigs map[string]string) error {
	// 检查直接路径冲�?
	pathToShareIds := make(map[string][]string)
	for sharePath, id := range shareConfigs {
		pathToShareIds[sharePath] = append(pathToShareIds[sharePath], id)
	}

	for sharePath, ids := range pathToShareIds {
		if len(ids) > 1 {
			return fmt.Errorf("路径冲突: 路径 '%s' 被多个不同的分享ID使用: %s",
				sharePath, strings.Join(ids, ", "))
		}
	}

	// 检查层次冲�?
	for path1, id1 := range shareConfigs {
		for path2, id2 := range shareConfigs {
			if path1 == path2 || id1 == id2 {
				continue
			}

			// 检查前缀冲突
			if strings.HasPrefix(path2, path1+"/") || strings.HasPrefix(path1, path2+"/") {
				return fmt.Errorf("路径冲突: 路径 '%s' (ID: %s) 与路�?'%s' (ID: %s) 存在层次冲突",
					path1, id1, path2, id2)
			}
		}
	}

	return nil
}

// 构建树形结构
func (d *DoubaoShare) _buildTreeStructure(shareConfigs map[string]string, rootShares []string) map[string]*RootFileList {
	rootMap := make(map[string]*RootFileList)

	// 添加所有分享节�?
	for sharePath, shareId := range shareConfigs {
		children := make([]RootFileList, 0)
		rootMap[sharePath] = &RootFileList{
			ShareID:     shareId,
			VirtualPath: sharePath,
			NodeInfo:    NodeInfoData{},
			Child:       &children,
		}
	}

	// 构建父子关系
	for sharePath, node := range rootMap {
		if sharePath == "" {
			continue
		}

		pathParts := strings.Split(sharePath, "/")
		if len(pathParts) > 1 {
			parentPath := strings.Join(pathParts[:len(pathParts)-1], "/")

			// 确保所有父级路径都已创�?
			_ensurePathExists(rootMap, parentPath)

			// 添加当前节点到父节点
			if parent, exists := rootMap[parentPath]; exists {
				*parent.Child = append(*parent.Child, *node)
			}
		}
	}

	return rootMap
}

// 提取顶级节点
func (d *DoubaoShare) _extractTopLevelNodes(rootMap map[string]*RootFileList, rootShares []string) []RootFileList {
	var topLevelNodes []RootFileList

	// 添加根目录分�?
	for _, shareId := range rootShares {
		children := make([]RootFileList, 0)
		topLevelNodes = append(topLevelNodes, RootFileList{
			ShareID:     shareId,
			VirtualPath: "",
			NodeInfo:    NodeInfoData{},
			Child:       &children,
		})
	}

	// 添加顶级目录
	for rootPath, node := range rootMap {
		if rootPath == "" {
			continue
		}

		isTopLevel := true
		pathParts := strings.Split(rootPath, "/")

		if len(pathParts) > 1 {
			parentPath := strings.Join(pathParts[:len(pathParts)-1], "/")
			if _, exists := rootMap[parentPath]; exists {
				isTopLevel = false
			}
		}

		if isTopLevel {
			topLevelNodes = append(topLevelNodes, *node)
		}
	}

	return topLevelNodes
}

// 确保路径存在，创建所有必要的中间节点
func _ensurePathExists(rootMap map[string]*RootFileList, path string) {
	if path == "" {
		return
	}

	// 如果路径已存在，不需要再处理
	if _, exists := rootMap[path]; exists {
		return
	}

	// 创建当前路径节点
	children := make([]RootFileList, 0)
	rootMap[path] = &RootFileList{
		ShareID:     "",
		VirtualPath: path,
		NodeInfo:    NodeInfoData{},
		Child:       &children,
	}

	// 处理父路�?
	pathParts := strings.Split(path, "/")
	if len(pathParts) > 1 {
		parentPath := strings.Join(pathParts[:len(pathParts)-1], "/")

		// 确保父路径存�?
		_ensurePathExists(rootMap, parentPath)

		// 将当前节点添加为父节点的子节�?
		if parent, exists := rootMap[parentPath]; exists {
			*parent.Child = append(*parent.Child, *rootMap[path])
		}
	}
}

// _extractShareId 从URL或直接ID中提取分享ID
func _extractShareId(input string) string {
	input = strings.TrimSpace(input)
	if strings.HasPrefix(input, "http") {
		regex := regexp.MustCompile(`/drive/s/([a-zA-Z0-9]+)`)
		if matches := regex.FindStringSubmatch(input); len(matches) > 1 {
			return matches[1]
		}
		return ""
	}
	return input // 直接返回ID
}

// _findRootFileByShareID 查找指定ShareID的配�?
func _findRootFileByShareID(rootFiles []RootFileList, shareID string) *RootFileList {
	for i, rf := range rootFiles {
		if rf.ShareID == shareID {
			return &rootFiles[i]
		}
		if rf.Child != nil && len(*rf.Child) > 0 {
			if found := _findRootFileByShareID(*rf.Child, shareID); found != nil {
				return found
			}
		}
	}
	return nil
}

// _findNodeByPath 查找指定路径的节�?
func _findNodeByPath(rootFiles []RootFileList, path string) *RootFileList {
	for i, rf := range rootFiles {
		if rf.VirtualPath == path {
			return &rootFiles[i]
		}
		if rf.Child != nil && len(*rf.Child) > 0 {
			if found := _findNodeByPath(*rf.Child, path); found != nil {
				return found
			}
		}
	}
	return nil
}

// _findShareByPath 根据路径查找分享和相对路�?
func _findShareByPath(rootFiles []RootFileList, path string) (*RootFileList, string) {
	// 完全匹配或子路径匹配
	for i, rf := range rootFiles {
		if rf.VirtualPath == path {
			return &rootFiles[i], ""
		}

		if rf.VirtualPath != "" && strings.HasPrefix(path, rf.VirtualPath+"/") {
			relPath := strings.TrimPrefix(path, rf.VirtualPath+"/")

			// 先检查子节点
			if rf.Child != nil && len(*rf.Child) > 0 {
				if child, childPath := _findShareByPath(*rf.Child, path); child != nil {
					return child, childPath
				}
			}

			return &rootFiles[i], relPath
		}

		// 递归检查子节点
		if rf.Child != nil && len(*rf.Child) > 0 {
			if child, childPath := _findShareByPath(*rf.Child, path); child != nil {
				return child, childPath
			}
		}
	}

	// 检查根目录分享
	for i, rf := range rootFiles {
		if rf.VirtualPath == "" && rf.ShareID != "" {
			parts := strings.SplitN(path, "/", 2)
			if len(parts) > 0 && parts[0] == rf.ShareID {
				if len(parts) > 1 {
					return &rootFiles[i], parts[1]
				}
				return &rootFiles[i], ""
			}
		}
	}

	return nil, ""
}

// _findShareAndPath 根据给定路径查找对应的ShareID和相对路�?
func (d *DoubaoShare) _findShareAndPath(dir model.Obj) (string, string, error) {
	dirPath := dir.GetPath()

	// 如果是根目录，返回空值表示需要列出所有分�?
	if dirPath == "/" || dirPath == "" {
		return "", "", nil
	}

	// 检查是否是 FileObject 类型，并获取 ShareID
	if fo, ok := dir.(*FileObject); ok && fo.ShareID != "" {
		// 直接使用对象中存储的 ShareID
		// 计算相对路径（移除前导斜杠）
		relativePath := strings.TrimPrefix(dirPath, "/")

		// 递归查找对应�?RootFile
		found := _findRootFileByShareID(d.RootFiles, fo.ShareID)
		if found != nil {
			if found.VirtualPath != "" {
				// 如果此分享配置了路径前缀，需要考虑相对路径的计�?
				if strings.HasPrefix(relativePath, found.VirtualPath) {
					return fo.ShareID, strings.TrimPrefix(relativePath, found.VirtualPath+"/"), nil
				}
			}
			return fo.ShareID, relativePath, nil
		}

		// 如果找不到对应的 RootFile 配置，仍然使用对象中�?ShareID
		return fo.ShareID, relativePath, nil
	}

	// 移除开头的斜杠
	cleanPath := strings.TrimPrefix(dirPath, "/")

	// 先检查是否有直接匹配的根目录分享
	for _, rootFile := range d.RootFiles {
		if rootFile.VirtualPath == "" && rootFile.ShareID != "" {
			// 检查是否匹配当前路径的第一部分
			parts := strings.SplitN(cleanPath, "/", 2)
			if len(parts) > 0 && parts[0] == rootFile.ShareID {
				if len(parts) > 1 {
					return rootFile.ShareID, parts[1], nil
				}
				return rootFile.ShareID, "", nil
			}
		}
	}

	// 查找匹配此路径的分享或虚拟目�?
	share, relPath := _findShareByPath(d.RootFiles, cleanPath)
	if share != nil {
		return share.ShareID, relPath, nil
	}

	log.Warnf("[doubao_share] No matching share path found: %s", dirPath)
	return "", "", fmt.Errorf("no matching share path found: %s", dirPath)
}

// convertToFileObject 将File转换为FileObject
func (d *DoubaoShare) convertToFileObject(file File, shareId string, relativePath string) *FileObject {
	// 构建文件对象
	obj := &FileObject{
		Object: model.Object{
			ID:       file.ID,
			Name:     file.Name,
			Size:     file.Size,
			Modified: time.Unix(file.UpdateTime, 0),
			Ctime:    time.Unix(file.CreateTime, 0),
			IsFolder: file.NodeType == DirectoryType,
			Path:     path.Join(relativePath, file.Name),
		},
		ShareID:  shareId,
		Key:      file.Key,
		NodeID:   file.ID,
		NodeType: file.NodeType,
	}

	return obj
}

// getFilesInPath 获取指定分享和路径下的文�?
func (d *DoubaoShare) getFilesInPath(ctx context.Context, shareId, nodeId, relativePath string) ([]model.Obj, error) {
	var (
		files []File
		err   error
	)

	// 调用overview接口获取分享链接信息 nodeId
	if nodeId == "" {
		files, err = d.getShareOverview(shareId, "")
		if err != nil {
			return nil, fmt.Errorf("failed to get share link information: %w", err)
		}

		result := make([]model.Obj, 0, len(files))
		for _, file := range files {
			result = append(result, d.convertToFileObject(file, shareId, "/"))
		}

		return result, nil

	} else {
		files, err = d.getFiles(shareId, nodeId, "")
		if err != nil {
			return nil, fmt.Errorf("failed to get share file: %w", err)
		}

		result := make([]model.Obj, 0, len(files))
		for _, file := range files {
			result = append(result, d.convertToFileObject(file, shareId, path.Join("/", relativePath)))
		}

		return result, nil
	}
}

// listRootDirectory 处理根目录的内容展示
func (d *DoubaoShare) listRootDirectory(ctx context.Context) ([]model.Obj, error) {
	objects := make([]model.Obj, 0)

	// 分组处理：直接显示的分享内容 vs 虚拟目录
	var directShareIDs []string
	addedDirs := make(map[string]bool)

	// 处理所有根节点
	for _, rootFile := range d.RootFiles {
		if rootFile.VirtualPath == "" && rootFile.ShareID != "" {
			// 无路径分享，记录ShareID以便后续获取内容
			directShareIDs = append(directShareIDs, rootFile.ShareID)
		} else {
			// 有路径的分享，显示第一级目�?
			parts := strings.SplitN(rootFile.VirtualPath, "/", 2)
			firstLevel := parts[0]

			// 避免重复添加同名目录
			if _, exists := addedDirs[firstLevel]; exists {
				continue
			}

			// 创建虚拟目录对象
			obj := &FileObject{
				Object: model.Object{
					ID:       "",
					Name:     firstLevel,
					Modified: time.Now(),
					Ctime:    time.Now(),
					IsFolder: true,
					Path:     path.Join("/", firstLevel),
				},
				ShareID:  rootFile.ShareID,
				Key:      "",
				NodeID:   "",
				NodeType: DirectoryType,
			}
			objects = append(objects, obj)
			addedDirs[firstLevel] = true
		}
	}

	// 处理直接显示的分享内�?
	for _, shareID := range directShareIDs {
		shareFiles, err := d.getFilesInPath(ctx, shareID, "", "")
		if err != nil {
			log.Warnf("[doubao_share] Failed to get list of files in share %s: %s", shareID, err)
			continue
		}
		objects = append(objects, shareFiles...)
	}

	return objects, nil
}

// listVirtualDirectoryContent 列出虚拟目录的内�?
func (d *DoubaoShare) listVirtualDirectoryContent(dir model.Obj) ([]model.Obj, error) {
	dirPath := strings.TrimPrefix(dir.GetPath(), "/")
	objects := make([]model.Obj, 0)

	// 递归查找此路径的节点
	node := _findNodeByPath(d.RootFiles, dirPath)

	if node != nil && node.Child != nil {
		// 显示此节点的所有子节点
		for _, child := range *node.Child {
			// 计算显示名称（取路径的最后一部分�?
			displayName := child.VirtualPath
			if child.VirtualPath != "" {
				parts := strings.Split(child.VirtualPath, "/")
				displayName = parts[len(parts)-1]
			} else if child.ShareID != "" {
				displayName = child.ShareID
			}

			obj := &FileObject{
				Object: model.Object{
					ID:       "",
					Name:     displayName,
					Modified: time.Now(),
					Ctime:    time.Now(),
					IsFolder: true,
					Path:     path.Join("/", child.VirtualPath),
				},
				ShareID:  child.ShareID,
				Key:      "",
				NodeID:   "",
				NodeType: DirectoryType,
			}
			objects = append(objects, obj)
		}
	}

	return objects, nil
}
