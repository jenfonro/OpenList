package handles

import (
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/OpenListTeam/OpenList/v4/internal/conf"
	"github.com/OpenListTeam/OpenList/v4/internal/db"
	"github.com/OpenListTeam/OpenList/v4/internal/model"
	"github.com/OpenListTeam/OpenList/v4/internal/task"

	"github.com/OpenListTeam/OpenList/v4/internal/fs"
	"github.com/OpenListTeam/OpenList/v4/internal/offline_download/tool"
	"github.com/OpenListTeam/OpenList/v4/pkg/utils"
	"github.com/OpenListTeam/OpenList/v4/server/common"
	"github.com/OpenListTeam/tache"
	"github.com/gin-gonic/gin"
)

type TaskInfo struct {
	ID          string      `json:"id"`
	Name        string      `json:"name"`
	Creator     string      `json:"creator"`
	CreatorRole int         `json:"creator_role"`
	State       tache.State `json:"state"`
	Status      string      `json:"status"`
	Progress    float64     `json:"progress"`
	StartTime   *time.Time  `json:"start_time"`
	EndTime     *time.Time  `json:"end_time"`
	TotalBytes  int64       `json:"total_bytes"`
	Error       string      `json:"error"`
}

const (
	defaultTaskPageSize = 20
	maxTaskPageSize     = 200
)

var undoneStates = []tache.State{
	tache.StatePending,
	tache.StateRunning,
	tache.StateCanceling,
	tache.StateErrored,
	tache.StateFailing,
	tache.StateWaitingRetry,
	tache.StateBeforeRetry,
}

var doneStates = []tache.State{
	tache.StateCanceled,
	tache.StateFailed,
	tache.StateSucceeded,
}

type taskListQuery struct {
	page     int
	pageSize int
	orderBy  string
	reverse  bool
	mine     bool
	regex    *regexp.Regexp
	keyword  string
}

func getTaskInfo[T task.TaskExtensionInfo](task T) TaskInfo {
	errMsg := ""
	if task.GetErr() != nil {
		errMsg = task.GetErr().Error()
	}
	progress := task.GetProgress()
	// if progress is NaN, set it to 100
	if math.IsNaN(progress) {
		progress = 100
	}
	creatorName := ""
	creatorRole := -1
	if task.GetCreator() != nil {
		creatorName = task.GetCreator().Username
		creatorRole = task.GetCreator().Role
	}
	return TaskInfo{
		ID:          task.GetID(),
		Name:        task.GetName(),
		Creator:     creatorName,
		CreatorRole: creatorRole,
		State:       task.GetState(),
		Status:      task.GetStatus(),
		Progress:    progress,
		StartTime:   task.GetStartTime(),
		EndTime:     task.GetEndTime(),
		TotalBytes:  task.GetTotalBytes(),
		Error:       errMsg,
	}
}

func getTaskInfos[T task.TaskExtensionInfo](tasks []T) []TaskInfo {
	return utils.MustSliceConvert(tasks, getTaskInfo[T])
}

func argsContains[T comparable](v T, slice ...T) bool {
	return utils.SliceContains(slice, v)
}

func parseTaskListQuery(c *gin.Context) (taskListQuery, error) {
	page, err := strconv.Atoi(c.DefaultQuery("page", "1"))
	if err != nil || page < 1 {
		page = 1
	}
	pageSize, err := strconv.Atoi(c.DefaultQuery("page_size", strconv.Itoa(defaultTaskPageSize)))
	if err != nil || pageSize <= 0 {
		pageSize = defaultTaskPageSize
	}
	if pageSize > maxTaskPageSize {
		pageSize = maxTaskPageSize
	}
	orderBy := strings.ToLower(c.DefaultQuery("order_by", "name"))
	switch orderBy {
	case "name", "creator", "state", "progress":
	default:
		orderBy = "name"
	}
	order := strings.ToLower(c.DefaultQuery("order", ""))
	reverse := order == "desc" || order == "true"
	mine, _ := strconv.ParseBool(c.DefaultQuery("mine", "false"))
	var compiled *regexp.Regexp
	keyword := c.Query("regex")
	if reg := c.Query("regex"); reg != "" {
		r, err := regexp.Compile(reg)
		if err != nil {
			return taskListQuery{}, err
		}
		compiled = r
	}
	return taskListQuery{
		page:     page,
		pageSize: pageSize,
		orderBy:  orderBy,
		reverse:  reverse,
		mine:     mine,
		regex:    compiled,
		keyword:  keyword,
	}, nil
}

func taskProgressValue[T task.TaskExtensionInfo](task T) float64 {
	progress := task.GetProgress()
	if math.IsNaN(progress) {
		return 100
	}
	return progress
}

func creatorName[T task.TaskExtensionInfo](task T) string {
	if task.GetCreator() != nil {
		return task.GetCreator().Username
	}
	return ""
}

func compareString(a, b string) int {
	switch {
	case a == b:
		return 0
	case a > b:
		return 1
	default:
		return -1
	}
}

func compareState(a, b tache.State) int {
	switch {
	case a == b:
		return 0
	case a > b:
		return 1
	default:
		return -1
	}
}

func sortTasks[T task.TaskExtensionInfo](tasks []T, orderBy string, reverse bool) {
	sort.SliceStable(tasks, func(i, j int) bool {
		a := tasks[i]
		b := tasks[j]
		var cmp int
		switch orderBy {
		case "creator":
			cmp = compareString(creatorName(a), creatorName(b))
		case "state":
			cmp = compareState(a.GetState(), b.GetState())
		case "progress":
			pa := taskProgressValue(a)
			pb := taskProgressValue(b)
			switch {
			case pa == pb:
				cmp = compareString(a.GetID(), b.GetID())
			case pa > pb:
				cmp = -1
			default:
				cmp = 1
			}
		default:
			cmp = compareString(a.GetName(), b.GetName())
		}
		if cmp == 0 {
			cmp = compareString(a.GetID(), b.GetID())
		}
		if reverse {
			cmp = -cmp
		}
		return cmp < 0
	})
}

func recordsToInfos(records []model.TaskRecord) []TaskInfo {
	infos := make([]TaskInfo, 0, len(records))
	for i := range records {
		r := records[i]
		infos = append(infos, TaskInfo{
			ID:          r.TaskID,
			Name:        r.Name,
			Creator:     r.Creator,
			CreatorRole: r.CreatorRole,
			State:       tache.State(r.State),
			Status:      r.Status,
			Progress:    r.Progress,
			StartTime:   r.StartTime,
			EndTime:     r.EndTime,
			TotalBytes:  r.TotalBytes,
			Error:       r.Error,
		})
	}
	return infos
}

func taskListHandler[T task.TaskExtensionInfo](manager task.Manager[T], taskType string, useIndex bool, states ...tache.State) gin.HandlerFunc {
	return func(c *gin.Context) {
		isAdmin, uid, ok := getUserInfo(c)
		if !ok {
			common.ErrorStrResp(c, "user invalid", 401)
			return
		}
		query, err := parseTaskListQuery(c)
		if err != nil {
			common.ErrorStrResp(c, err.Error(), 400)
			return
		}
		restrictOwner := query.mine || !isAdmin
		if useIndex && query.regex == nil {
			creatorID := uint(0)
			if restrictOwner {
				creatorID = uid
			}
			records, total, err := db.ListTaskRecords(taskType, states, creatorID, query.keyword, query.page, query.pageSize)
			if err != nil {
				common.ErrorResp(c, err, 500, true)
				return
			}
			common.SuccessResp(c, common.PageResp{
			  Content: recordsToInfos(records),
			  Total:   total,
			})
				return
		}
		tasks := manager.GetByCondition(func(tsk T) bool {
			if !argsContains(tsk.GetState(), states...) {
				return false
			}
			creator := tsk.GetCreator()
			creatorID := uint(0)
			if creator != nil {
				creatorID = creator.ID
			}
			if !isAdmin && creatorID != uid {
				return false
			}
			if restrictOwner && creatorID != uid {
				return false
			}
			if query.regex != nil && !query.regex.MatchString(tsk.GetName()) {
				return false
			}
			return true
		})
		sortTasks(tasks, query.orderBy, query.reverse)
		total := len(tasks)
		start := (query.page - 1) * query.pageSize
		if start < 0 {
			start = 0
		}
		if start > total {
			start = total
		}
		end := start + query.pageSize
		if end > total {
			end = total
		}
		common.SuccessResp(c, common.PageResp{
			Content: getTaskInfos(tasks[start:end]),
			Total:   int64(total),
		})
	}
}

func getUserInfo(c *gin.Context) (bool, uint, bool) {
	if user, ok := c.Request.Context().Value(conf.UserKey).(*model.User); ok {
		return user.IsAdmin(), user.ID, true
	} else {
		return false, 0, false
	}
}

func getTargetedHandler[T task.TaskExtensionInfo](manager task.Manager[T], callback func(c *gin.Context, task T)) gin.HandlerFunc {
	return func(c *gin.Context) {
		isAdmin, uid, ok := getUserInfo(c)
		if !ok {
			// if there is no bug, here is unreachable
			common.ErrorStrResp(c, "user invalid", 401)
			return
		}
		t, ok := manager.GetByID(c.Query("tid"))
		if !ok {
			common.ErrorStrResp(c, "task not found", 404)
			return
		}
		if !isAdmin && uid != t.GetCreator().ID {
			// to avoid an attacker using error messages to guess valid TID, return a 404 rather than a 403
			common.ErrorStrResp(c, "task not found", 404)
			return
		}
		callback(c, t)
	}
}

func getBatchHandler[T task.TaskExtensionInfo](manager task.Manager[T], callback func(task T)) gin.HandlerFunc {
	return func(c *gin.Context) {
		isAdmin, uid, ok := getUserInfo(c)
		if !ok {
			common.ErrorStrResp(c, "user invalid", 401)
			return
		}
		var tids []string
		if err := c.ShouldBind(&tids); err != nil {
			common.ErrorStrResp(c, "invalid request format", 400)
			return
		}
		retErrs := make(map[string]string)
		for _, tid := range tids {
			t, ok := manager.GetByID(tid)
			if !ok || (!isAdmin && uid != t.GetCreator().ID) {
				retErrs[tid] = "task not found"
				continue
			}
			callback(t)
		}
		common.SuccessResp(c, retErrs)
	}
}

func taskRoute[T task.TaskExtensionInfo](g *gin.RouterGroup, manager task.Manager[T], taskType string, useIndex bool) {
	g.GET("/undone", taskListHandler(manager, taskType, useIndex, undoneStates...))
	g.GET("/done", taskListHandler(manager, taskType, useIndex, doneStates...))
	g.POST("/info", getTargetedHandler(manager, func(c *gin.Context, task T) {
		common.SuccessResp(c, getTaskInfo(task))
	}))
	g.POST("/cancel", getTargetedHandler(manager, func(c *gin.Context, task T) {
		manager.Cancel(task.GetID())
		common.SuccessResp(c)
	}))
	g.POST("/delete", getTargetedHandler(manager, func(c *gin.Context, task T) {
		manager.Remove(task.GetID())
		common.SuccessResp(c)
	}))
	g.POST("/retry", getTargetedHandler(manager, func(c *gin.Context, task T) {
		manager.Retry(task.GetID())
		common.SuccessResp(c)
	}))
	g.POST("/cancel_some", getBatchHandler(manager, func(task T) {
		manager.Cancel(task.GetID())
	}))
	g.POST("/delete_some", getBatchHandler(manager, func(task T) {
		manager.Remove(task.GetID())
	}))
	g.POST("/retry_some", getBatchHandler(manager, func(task T) {
		manager.Retry(task.GetID())
	}))
	g.POST("/clear_done", func(c *gin.Context) {
		isAdmin, uid, ok := getUserInfo(c)
		if !ok {
			// if there is no bug, here is unreachable
			common.ErrorStrResp(c, "user invalid", 401)
			return
		}
		manager.RemoveByCondition(func(task T) bool {
			return (isAdmin || uid == task.GetCreator().ID) &&
				argsContains(task.GetState(), tache.StateCanceled, tache.StateFailed, tache.StateSucceeded)
		})
		common.SuccessResp(c)
	})
	g.POST("/clear_succeeded", func(c *gin.Context) {
		isAdmin, uid, ok := getUserInfo(c)
		if !ok {
			// if there is no bug, here is unreachable
			common.ErrorStrResp(c, "user invalid", 401)
			return
		}
		manager.RemoveByCondition(func(task T) bool {
			return (isAdmin || uid == task.GetCreator().ID) && task.GetState() == tache.StateSucceeded
		})
		common.SuccessResp(c)
	})
	g.POST("/retry_failed", func(c *gin.Context) {
		isAdmin, uid, ok := getUserInfo(c)
		if !ok {
			// if there is no bug, here is unreachable
			common.ErrorStrResp(c, "user invalid", 401)
			return
		}
		tasks := manager.GetByCondition(func(task T) bool {
			return (isAdmin || uid == task.GetCreator().ID) && task.GetState() == tache.StateFailed
		})
		for _, t := range tasks {
			manager.Retry(t.GetID())
		}
		common.SuccessResp(c)
	})
}

func SetupTaskRoute(g *gin.RouterGroup) {
	taskRoute(g.Group("/upload"), fs.UploadTaskManager, "upload", false)
	taskRoute(g.Group("/copy"), fs.CopyTaskManager, "copy", true)
	taskRoute(g.Group("/move"), fs.MoveTaskManager, "move", true)
	taskRoute(g.Group("/offline_download"), tool.DownloadTaskManager, "download", true)
	taskRoute(g.Group("/offline_download_transfer"), tool.TransferTaskManager, "transfer", true)
	taskRoute(g.Group("/decompress"), fs.ArchiveDownloadTaskManager, "decompress", true)
	taskRoute(g.Group("/decompress_upload"), fs.ArchiveContentUploadTaskManager, "decompress_upload", false)
}
