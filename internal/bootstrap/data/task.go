package data

import (
	"github.com/OpenListTeam/OpenList/v4/internal/conf"
	"github.com/OpenListTeam/OpenList/v4/internal/db"
	"github.com/OpenListTeam/OpenList/v4/internal/model"
	"github.com/OpenListTeam/OpenList/v4/pkg/utils"
)

var initialTaskItems []model.TaskItem

func initTasks() {
	InitialTasks()

	for i := range initialTaskItems {
		item := &initialTaskItems[i]
		taskitem, _ := db.GetTaskDataByType(item.Key)
		if taskitem == nil {
			if err := db.CreateTaskData(item); err != nil {
				utils.Log.Warnf("failed to init task persist record for %s: %+v", item.Key, err)
			}
		}
	}
	clearDisabledTaskData()
}

func clearDisabledTaskData() {
	persistEnabled := map[string]bool{
		"copy":       conf.Conf.Tasks.Copy.TaskPersistant,
		"move":       conf.Conf.Tasks.Move.TaskPersistant,
		"download":   conf.Conf.Tasks.Download.TaskPersistant,
		"transfer":   conf.Conf.Tasks.Transfer.TaskPersistant,
		"decompress": conf.Conf.Tasks.Decompress.TaskPersistant,
	}
	for i := range initialTaskItems {
		item := &initialTaskItems[i]
		enabled, ok := persistEnabled[item.Key]
		if ok && !enabled {
			if err := db.UpdateTaskData(&model.TaskItem{Key: item.Key, PersistData: "[]"}); err != nil {
				utils.Log.Warnf("failed to clear task data for %s: %+v", item.Key, err)
			}
		}
	}
}

func InitialTasks() []model.TaskItem {
	initialTaskItems = []model.TaskItem{
		{Key: "copy", PersistData: "[]"},
		{Key: "move", PersistData: "[]"},
		{Key: "download", PersistData: "[]"},
		{Key: "transfer", PersistData: "[]"},
		{Key: "decompress", PersistData: "[]"},
	}
	return initialTaskItems
}
