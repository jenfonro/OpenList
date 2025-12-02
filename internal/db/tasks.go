package db

import (
	"encoding/json"
	"math"
	"time"

	"github.com/OpenListTeam/OpenList/v4/internal/conf"
	"github.com/OpenListTeam/OpenList/v4/internal/model"
	"github.com/OpenListTeam/OpenList/v4/internal/task"
	"github.com/OpenListTeam/OpenList/v4/pkg/utils"
	"github.com/OpenListTeam/tache"
	"github.com/pkg/errors"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func GetTaskDataByType(type_s string) (*model.TaskItem, error) {
	task := model.TaskItem{Key: type_s}
	if err := db.Where(task).First(&task).Error; err != nil {
		return nil, errors.Wrapf(err, "failed find task")
	}
	return &task, nil
}

func UpdateTaskData(t *model.TaskItem) error {
	return errors.WithStack(db.Model(&model.TaskItem{}).Where("key = ?", t.Key).Update("persist_data", t.PersistData).Error)
}

func CreateTaskData(t *model.TaskItem) error {
	return errors.WithStack(db.Create(t).Error)
}

func GetTaskDataFunc(type_s string, enabled bool) func() ([]byte, error) {
	if !enabled {
		return nil
	}
	task, err := GetTaskDataByType(type_s)
	if err != nil {
		return nil
	}
	return func() ([]byte, error) {
		<-conf.StoragesLoadSignal()
		return []byte(task.PersistData), nil
	}
}

func UpdateTaskDataFunc(type_s string, enabled bool) func([]byte) error {
	if !enabled {
		return nil
	}
	return func(data []byte) error {
		s := string(data)
		if s == "null" || s == "" {
			s = "[]"
		}
		return UpdateTaskData(&model.TaskItem{Key: type_s, PersistData: s})
	}
}

// GetTaskPersistReadFunc returns a non-nil reader so we can still trigger indexing when persistence is disabled.
func GetTaskPersistReadFunc(type_s string, persistEnabled bool) func() ([]byte, error) {
	if persistEnabled {
		return GetTaskDataFunc(type_s, true)
	}
	return func() ([]byte, error) {
		return []byte("[]"), nil
	}
}

type TaskRecordLite struct {
	ID          string
	Name        string
	Creator     string
	CreatorID   uint
	CreatorRole int
	State       tache.State
	Status      string
	Progress    float64
	StartTime   *time.Time
	EndTime     *time.Time
	TotalBytes  int64
	Error       string
}

func convertToRecord(taskType string, t TaskRecordLite) model.TaskRecord {
	progress := t.Progress
	if math.IsNaN(progress) {
		progress = 100
	}
	return model.TaskRecord{
		TaskID:      t.ID,
		Type:        taskType,
		Name:        t.Name,
		Creator:     t.Creator,
		CreatorID:   t.CreatorID,
		CreatorRole: t.CreatorRole,
		State:       int(t.State),
		Status:      t.Status,
		Progress:    progress,
		StartTime:   t.StartTime,
		EndTime:     t.EndTime,
		TotalBytes:  t.TotalBytes,
		Error:       t.Error,
	}
}

func toLite[T task.TaskExtensionInfo](task T) TaskRecordLite {
	var creatorName string
	var creatorID uint
	creatorRole := -1
	if creator := task.GetCreator(); creator != nil {
		creatorName = creator.Username
		creatorID = creator.ID
		creatorRole = creator.Role
	}
	return TaskRecordLite{
		ID:          task.GetID(),
		Name:        task.GetName(),
		Creator:     creatorName,
		CreatorID:   creatorID,
		CreatorRole: creatorRole,
		State:       task.GetState(),
		Status:      task.GetStatus(),
		Progress:    task.GetProgress(),
		StartTime:   task.GetStartTime(),
		EndTime:     task.GetEndTime(),
		TotalBytes:  task.GetTotalBytes(),
		Error: func() string {
			if task.GetErr() == nil {
				return ""
			}
			return task.GetErr().Error()
		}(),
	}
}

func ReplaceTaskRecords(taskType string, records []model.TaskRecord) error {
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("type = ?", taskType).Delete(&model.TaskRecord{}).Error; err != nil {
			return errors.WithStack(err)
		}
		if len(records) == 0 {
			return nil
		}
		return errors.WithStack(tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "task_id"}, {Name: "type"}},
			DoUpdates: clause.AssignmentColumns([]string{"name", "creator", "creator_id", "creator_role", "state", "status", "progress", "start_time", "end_time", "total_bytes", "error", "updated_at"}),
		}).CreateInBatches(records, 500).Error)
	})
}

func UpsertTaskRecordsFromTasks[T task.TaskExtensionInfo](taskType string, tasks []T) error {
	records := make([]model.TaskRecord, 0, len(tasks))
	for i := range tasks {
		records = append(records, convertToRecord(taskType, toLite(tasks[i])))
	}
	return ReplaceTaskRecords(taskType, records)
}

func UpdateTaskDataAndIndexFunc[T task.TaskExtensionInfo](type_s string, persistEnabled bool) func([]byte) error {
	return func(data []byte) error {
		content := string(data)
		if content == "null" || content == "" {
			content = "[]"
		}
		if persistEnabled {
			if err := UpdateTaskData(&model.TaskItem{Key: type_s, PersistData: content}); err != nil {
				return err
			}
		}

		var tasks []T
		if err := json.Unmarshal([]byte(content), &tasks); err != nil {
			utils.Log.Warnf("failed to unmarshal tasks for indexing, type=%s: %+v", type_s, err)
			return nil
		}
		if err := UpsertTaskRecordsFromTasks(type_s, tasks); err != nil {
			utils.Log.Warnf("failed to update task index, type=%s: %+v", type_s, err)
		}
		return nil
	}
}

func ListTaskRecords(taskType string, states []tache.State, creatorID uint, keyword string, page, pageSize int) ([]model.TaskRecord, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	offset := (page - 1) * pageSize
	tx := db.Model(&model.TaskRecord{}).Where("type = ?", taskType)
	if len(states) > 0 {
		tx = tx.Where("state IN ?", states)
	}
	if creatorID != 0 {
		tx = tx.Where("creator_id = ?", creatorID)
	}
	if keyword != "" {
		tx = tx.Where("name LIKE ?", "%"+keyword+"%")
	}
	var total int64
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, errors.WithStack(err)
	}
	var records []model.TaskRecord
	err := tx.Order("COALESCE(end_time, start_time) DESC").
		Order("task_id DESC").
		Offset(offset).
		Limit(pageSize).
		Find(&records).Error
	return records, total, errors.WithStack(err)
}

func DeleteTaskRecordsByType(taskType string) error {
	return errors.WithStack(db.Where("type = ?", taskType).Delete(&model.TaskRecord{}).Error)
}
