package model

import "time"

// TaskRecord stores task list snapshot for indexed pagination.
type TaskRecord struct {
	TaskID      string     `gorm:"column:task_id;primaryKey;size:64" json:"task_id"`
	Type        string     `gorm:"column:type;primaryKey;size:64;index:idx_task_type_state" json:"type"`
	Name        string     `gorm:"column:name;size:1024" json:"name"`
	Creator     string     `gorm:"column:creator;size:255;index:idx_task_creator" json:"creator"`
	CreatorID   uint       `gorm:"column:creator_id;index:idx_task_creator_id" json:"creator_id"`
	CreatorRole int        `gorm:"column:creator_role" json:"creator_role"`
	State       int        `gorm:"column:state;index:idx_task_type_state" json:"state"`
	Status      string     `gorm:"column:status;size:255" json:"status"`
	Progress    float64    `gorm:"column:progress" json:"progress"`
	StartTime   *time.Time `gorm:"column:start_time;index:idx_task_start_time" json:"start_time"`
	EndTime     *time.Time `gorm:"column:end_time;index:idx_task_end_time" json:"end_time"`
	TotalBytes  int64      `gorm:"column:total_bytes" json:"total_bytes"`
	Error       string     `gorm:"column:error;size:1024" json:"error"`
	UpdatedAt   time.Time  `gorm:"column:updated_at;autoUpdateTime" json:"updated_at"`
	CreatedAt   time.Time  `gorm:"column:created_at;autoCreateTime" json:"created_at"`
}

func (TaskRecord) TableName() string {
	return "task_records"
}
