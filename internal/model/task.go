package model

import "time"

type TaskItem struct {
	Key         string `json:"key"`
	PersistData string `gorm:"type:text" json:"persist_data"`
}

// TaskPersist stores each task snapshot independently to avoid giant JSON blobs.
// Key distinguishes task type (copy/move/upload...), TaskID is the manager task ID.
type TaskPersist struct {
	ID        uint      `gorm:"primaryKey"`
	Key       string    `gorm:"index:idx_task_key_id"`
	TaskID    string    `gorm:"index:idx_task_key_id" json:"task_id"`
	Data      string    `gorm:"type:longtext" json:"data"`
	CreatedAt time.Time `json:"-" gorm:"autoCreateTime"`
	UpdatedAt time.Time `json:"-" gorm:"autoUpdateTime"`
}
