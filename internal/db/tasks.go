package db

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/OpenListTeam/OpenList/v4/internal/conf"
	"github.com/OpenListTeam/OpenList/v4/internal/model"
	"github.com/pkg/errors"
	"gorm.io/gorm"
)

// per-task persistence to avoid giant single-json blobs
type taskRow struct {
	Key    string
	TaskID string
	Data   string
}

// GetTaskDataFunc returns all persisted tasks for the given type as a JSON array []byte.
func GetTaskDataFunc(type_s string, enabled bool) func() ([]byte, error) {
	if !enabled {
		return nil
	}
	return func() ([]byte, error) {
		<-conf.StoragesLoadSignal()
		var records []model.TaskPersist
		if err := db.Where("key = ?", type_s).Order("updated_at desc").Find(&records).Error; err != nil {
			return nil, errors.Wrapf(err, "failed find task rows")
		}
		if len(records) == 0 {
			return []byte("[]"), nil
		}
		var buf bytes.Buffer
		buf.WriteByte('[')
		for i, r := range records {
			if i > 0 {
				buf.WriteByte(',')
			}
			buf.WriteString(r.Data)
		}
		buf.WriteByte(']')
		return buf.Bytes(), nil
	}
}

// Legacy helpers for old TaskItem table (kept minimal for initTasks)
func GetTaskItemByType(type_s string) (*model.TaskItem, error) {
	task := model.TaskItem{Key: type_s}
	if err := db.Where(task).First(&task).Error; err != nil {
		return nil, errors.Wrapf(err, "failed find task")
	}
	return &task, nil
}

func CreateTaskItem(t *model.TaskItem) error {
	return errors.WithStack(db.Create(t).Error)
}

// UpdateTaskDataFunc upserts task rows per type; snapshot replace strategy.
func UpdateTaskDataFunc(type_s string, enabled bool) func([]byte) error {
	if !enabled {
		return nil
	}
	return func(data []byte) error {
		trimmed := strings.TrimSpace(string(data))
		if trimmed == "" || trimmed == "null" {
			return db.Where("key = ?", type_s).Delete(&model.TaskPersist{}).Error
		}
		var raws []json.RawMessage
		if err := json.Unmarshal(data, &raws); err != nil {
			return errors.Wrap(err, "failed parse task snapshot")
		}

		rows := make([]model.TaskPersist, 0, len(raws))
		for i, raw := range raws {
			var tmp struct {
				ID     string `json:"id"`
				TaskID string `json:"task_id"`
			}
			_ = json.Unmarshal(raw, &tmp)
			taskID := tmp.ID
			if taskID == "" {
				taskID = tmp.TaskID
			}
			if taskID == "" {
				taskID = fmt.Sprintf("%s-%d", type_s, i)
			}
			rows = append(rows, model.TaskPersist{
				Key:    type_s,
				TaskID: taskID,
				Data:   string(raw),
			})
		}

		return db.Transaction(func(tx *gorm.DB) error {
			if err := tx.Where("key = ?", type_s).Delete(&model.TaskPersist{}).Error; err != nil {
				return err
			}
			if len(rows) > 0 {
				if err := tx.Create(&rows).Error; err != nil {
					return err
				}
			}
			return nil
		})
	}
}
